package peer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Uhm-why/caduceus/internal/compactor"
	"github.com/Uhm-why/caduceus/internal/mailbox"
)

// TaskManager defines the interface for creating, updating, and querying delegated tasks.
type TaskManager interface {
	// Dispatch accepts a new task and registers its cancellation handle.
	Dispatch(req TaskDispatchRequest, cancelFn context.CancelFunc) (*TaskDispatchResponse, error)
	// Get retrieves a task by its unique identifier.
	Get(taskID string) (*Task, bool)
	// UpdateProgress updates the progress description of an in-flight task.
	UpdateProgress(taskID string, progress string) error
	// Complete marks a task as successfully finished with result text and artifact paths.
	Complete(taskID string, result string, artifacts []string) error
	// Fail records that a task encountered an unrecoverable failure.
	Fail(taskID string, failureErr error) error
	// Cancel requests termination of an active task.
	Cancel(taskID string, reason string) (*TaskCancelResponse, error)
	// ListActive returns all tasks currently in Queued or Running state.
	ListActive() []*Task
}

// DefaultTaskManager provides a thread-safe in-memory implementation of TaskManager
// that publishes task lifecycle events to a Mailbox.
type DefaultTaskManager struct {
	tasks     map[string]*Task
	mu        sync.RWMutex
	mailbox   mailbox.Mailbox
	compactor *compactor.Compactor
}

// NewTaskManager constructs a DefaultTaskManager using mb for event notification persistence.
func NewTaskManager(mb mailbox.Mailbox) *DefaultTaskManager {
	return &DefaultTaskManager{
		tasks:   make(map[string]*Task),
		mailbox: mb,
	}
}

// SetCompactor attaches a context Compactor to reduce large result payloads upon task completion.
func (tm *DefaultTaskManager) SetCompactor(c *compactor.Compactor) {
	tm.compactor = c
}

// Dispatch registers a new task, stores its cancellation callback, and records a dispatch event.
func (tm *DefaultTaskManager) Dispatch(req TaskDispatchRequest, cancelFn context.CancelFunc) (*TaskDispatchResponse, error) {
	tm.mu.Lock()
	if _, exists := tm.tasks[req.TaskID]; exists {
		tm.mu.Unlock()
		return &TaskDispatchResponse{
			TaskID:   req.TaskID,
			Accepted: false,
			Reason:   "task already exists",
		}, nil
	}

	task := &Task{
		ID:        req.TaskID,
		Objective: req.Objective,
		Context:   req.Context,
		State:     TaskStateRunning,
		CreatedAt: time.Now().UTC(),
		cancelFn:  cancelFn,
	}

	tm.tasks[req.TaskID] = task

	tm.mu.Unlock()

	if tm.mailbox != nil {
		tm.mailbox.Enqueue(mailbox.Message{
			Kind:          "task_dispatch",
			CorrelationID: req.TaskID,
			Content:       req.Objective,
			Timestamp:     time.Now().UTC(),
		})
	}

	return &TaskDispatchResponse{
		TaskID:       req.TaskID,
		Accepted:     true,
		InitialState: "running",
	}, nil
}

// Get returns the task corresponding to taskID, or false if not found.
func (tm *DefaultTaskManager) Get(taskID string) (*Task, bool) {
	tm.mu.RLock()
	task, ok := tm.tasks[taskID]
	tm.mu.RUnlock()
	return task, ok
}

// UpdateProgress updates the progress text of an active task.
func (tm *DefaultTaskManager) UpdateProgress(taskID string, progress string) error {
	task, ok := tm.Get(taskID)
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.mu.Lock()
	task.Progress = progress
	task.mu.Unlock()

	return nil
}

// Complete sets the task state to Completed, compacts the result if configured,
// and publishes a task_result message to the mailbox.
func (tm *DefaultTaskManager) Complete(taskID string, result string, artifacts []string) error {
	task, ok := tm.Get(taskID)
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	finalResult := result
	if tm.compactor != nil {
		if compacted, err := tm.compactor.Compact(result, taskID); err == nil {
			finalResult = compacted
		}
	}

	task.mu.Lock()
	task.State = TaskStateCompleted
	task.Result = finalResult
	task.Artifacts = artifacts
	task.CompletedAt = time.Now().UTC()
	task.mu.Unlock()

	if tm.mailbox != nil {
		tm.mailbox.Enqueue(mailbox.Message{
			Kind:          "task_result",
			CorrelationID: taskID,
			Content:       finalResult,
			Timestamp:     time.Now().UTC(),
		})
	}

	return nil
}

// Fail marks the task as Failed with failureErr, recording a task_error in the mailbox.
func (tm *DefaultTaskManager) Fail(taskID string, failureErr error) error {
	task, ok := tm.Get(taskID)
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.mu.Lock()
	task.State = TaskStateFailed
	task.ErrorMsg = failureErr.Error()
	task.CompletedAt = time.Now().UTC()
	task.mu.Unlock()
	if tm.mailbox != nil {
		tm.mailbox.Enqueue(mailbox.Message{
			Kind:          "task_error",
			CorrelationID: taskID,
			Content:       failureErr.Error(),
			Timestamp:     time.Now().UTC(),
		})
	}

	return nil
}

// Cancel terminates an active task using its cancel function and transitions its state to Aborted.
func (tm *DefaultTaskManager) Cancel(taskID string, reason string) (*TaskCancelResponse, error) {
	task, ok := tm.Get(taskID)
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	task.mu.Lock()
	if task.State == TaskStateCompleted || task.State == TaskStateFailed || task.State == TaskStateAborted {
		final := task.State.String()
		task.mu.Unlock()
		return &TaskCancelResponse{
			TaskID:     taskID,
			Canceled:   false,
			FinalState: final,
		}, nil
	}

	if task.cancelFn != nil {
		task.cancelFn()
	}

	task.State = TaskStateAborted
	task.CompletedAt = time.Now().UTC()
	task.mu.Unlock()

	if tm.mailbox != nil {
		tm.mailbox.Enqueue(mailbox.Message{
			Kind:          "task_abort",
			CorrelationID: taskID,
			Content:       reason,
			Timestamp:     time.Now().UTC(),
		})
	}

	return &TaskCancelResponse{
		TaskID:     taskID,
		Canceled:   true,
		FinalState: "aborted",
	}, nil
}

// ListActive returns all tasks currently in Queued or Running state.
func (tm *DefaultTaskManager) ListActive() []*Task {
	tm.mu.RLock()
	var active []*Task
	for _, task := range tm.tasks {
		task.mu.RLock()
		if task.State == TaskStateQueued || task.State == TaskStateRunning {
			active = append(active, task)
		}
		task.mu.RUnlock()
	}
	tm.mu.RUnlock()
	return active
}
