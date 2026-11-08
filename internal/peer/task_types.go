package peer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// TaskState represents the execution lifecycle state of a delegated task.
type TaskState int

// Discrete lifecycle states for delegated tasks.
const (
	TaskStateUnknown   TaskState = iota // Uninitialized or unknown state.
	TaskStateQueued                     // Task accepted and awaiting execution.
	TaskStateRunning                    // Task actively executing.
	TaskStateCompleted                  // Task finished successfully.
	TaskStateFailed                     // Task failed with an error.
	TaskStateAborted                    // Task canceled or aborted.
)

var taskStateName = map[TaskState]string{
	TaskStateUnknown:   "unknown",
	TaskStateQueued:    "queued",
	TaskStateRunning:   "running",
	TaskStateCompleted: "completed",
	TaskStateFailed:    "failed",
	TaskStateAborted:   "aborted",
}

var taskStateValue = make(map[string]TaskState)

func init() {
	for state, name := range taskStateName {
		taskStateValue[name] = state
	}
}

// MarshalJSON serializes TaskState to its JSON string representation.
func (ts TaskState) MarshalJSON() ([]byte, error) {
	marshaled, err := json.Marshal(ts.String())
	return marshaled, err
}

// UnmarshalJSON parses a JSON string representation into TaskState.
func (ts *TaskState) UnmarshalJSON(data []byte) error {
	var s string

	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	state, ok := taskStateValue[s]

	if !ok {
		return fmt.Errorf("unknown task state: %q", s)
	}

	*ts = state

	return nil

}

// String returns the string representation of TaskState.
func (ts TaskState) String() string {
	if name, ok := taskStateName[ts]; ok {
		return name
	}

	return taskStateName[TaskStateUnknown]
}

// TaskDispatchRequest defines the parameters for delegating a task to a peer agent.
type TaskDispatchRequest struct {
	TaskID     string `json:"task_id"`
	Objective  string `json:"objective"`
	Context    string `json:"context"`
	TimeoutSec int    `json:"timeout_sec"`
	Priority   int    `json:"priority,omitempty"`
}

// TaskDispatchResponse defines the peer's immediate acceptance or rejection of a task dispatch.
type TaskDispatchResponse struct {
	TaskID       string `json:"task_id"`
	Accepted     bool   `json:"accepted"`
	Reason       string `json:"reason,omitempty"`
	InitialState string `json:"initial_state,omitempty"`
}

// TaskStatusRequest queries execution progress and results for a specific task ID.
type TaskStatusRequest struct {
	TaskID string `json:"task_id"`
}

// TaskStatusResponse reports detailed status, progress, results, or error messages for a task.
type TaskStatusResponse struct {
	TaskID     string   `json:"task_id"`
	State      string   `json:"state"`
	Progress   string   `json:"progress"`
	Result     string   `json:"result"`
	Artifacts  []string `json:"artifacts,omitempty"`
	Error      string   `json:"error,omitempty"`
	ElapsedSec int64    `json:"elapsed_sec"`
}

// TaskCancelRequest specifies the task ID and reason for canceling an in-flight task.
type TaskCancelRequest struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

// TaskCancelResponse reports the result and final state of a task cancellation request.
type TaskCancelResponse struct {
	TaskID     string `json:"task_id"`
	Canceled   bool   `json:"canceled"`
	FinalState string `json:"final_state"`
}

// Task represents an in-memory task entity managed by TaskManager.
type Task struct {
	ID          string             `json:"id"`
	Objective   string             `json:"objective"`
	Context     string             `json:"context"`
	State       TaskState          `json:"task_state"`
	Progress    string             `json:"progress"`
	Result      string             `json:"result,omitempty"`
	Artifacts   []string           `json:"artifacts,omitempty"`
	ErrorMsg    string             `json:"error_msg,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	StartedAt   time.Time          `json:"started_at,omitempty"`
	CompletedAt time.Time          `json:"completed_at,omitempty"`
	cancelFn    context.CancelFunc `json:"-"`
	mu          sync.RWMutex       `json:"-"`
}
