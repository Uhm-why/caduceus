package peer

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Uhm-why/caduceus/internal/mailbox"
)

func TestTaskManager_DispatchAndDuplicate(t *testing.T) {
	mb, err := mailbox.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to initialize test mailbox: %v", err)
	}

	tm := NewTaskManager(mb)

	req := TaskDispatchRequest{
		TaskID:     "task-001",
		Objective:  "Inspect harness",
		Context:    "Bench 1",
		TimeoutSec: 30,
	}

	res, err := tm.Dispatch(req, nil)
	if err != nil {
		t.Fatalf("unexpected error on dispatch: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("expected task to be accepted, got false")
	}
	if res.InitialState != "running" {
		t.Fatalf("expected initial state 'running', got %q", res.InitialState)
	}

	task, ok := tm.Get("task-001")
	if !ok || task == nil {
		t.Fatalf("expected to retrieve task-001, got ok=%v", ok)
	}
	if task.State != TaskStateRunning {
		t.Fatalf("expected state %v, got %v", TaskStateRunning, task.State)
	}

	resDup, err := tm.Dispatch(req, nil)
	if err != nil {
		t.Fatalf("unexpected error on duplicate dispatch: %v", err)
	}
	if resDup.Accepted {
		t.Fatalf("expected duplicate dispatch to be rejected, got accepted=true")
	}
	if resDup.Reason != "task already exists" {
		t.Fatalf("expected reason 'task already exists', got %q", resDup.Reason)
	}
}

func TestTaskManager_LifecycleComplete(t *testing.T) {
	mb, err := mailbox.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to initialize test mailbox: %v", err)
	}

	tm := NewTaskManager(mb)

	req := TaskDispatchRequest{
		TaskID:     "task-002",
		Objective:  "Solder replacement IC",
		Context:    "Microscope station",
		TimeoutSec: 60,
	}

	_, err = tm.Dispatch(req, nil)
	if err != nil {
		t.Fatalf("failed to dispatch task: %v", err)
	}

	if err := tm.UpdateProgress("task-002", "pads tinned"); err != nil {
		t.Fatalf("failed to update progress: %v", err)
	}

	task, ok := tm.Get("task-002")
	if !ok {
		t.Fatalf("task-002 not found")
	}
	if task.Progress != "pads tinned" {
		t.Fatalf("expected progress 'pads tinned', got %q", task.Progress)
	}

	artifacts := []string{"/tmp/diagnostic.png", "/tmp/log.txt"}
	if err := tm.Complete("task-002", "reflow successful", artifacts); err != nil {
		t.Fatalf("failed to complete task: %v", err)
	}

	if task.State != TaskStateCompleted {
		t.Fatalf("expected state %v, got %v", TaskStateCompleted, task.State)
	}
	if task.Result != "reflow successful" {
		t.Fatalf("expected result 'reflow successful', got %q", task.Result)
	}
	if len(task.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(task.Artifacts))
	}
	if task.CompletedAt.IsZero() {
		t.Fatalf("expected CompletedAt to be set")
	}

	msgs, err := mb.Drain()
	if err != nil {
		t.Fatalf("failed to drain mailbox: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 mailbox messages (dispatch + result), got %d", len(msgs))
	}

	if msgs[0].Kind != "task_dispatch" || msgs[0].CorrelationID != "task-002" {
		t.Fatalf("unexpected dispatch msg: %+v", msgs[0])
	}
	if msgs[1].Kind != "task_result" || msgs[1].CorrelationID != "task-002" || msgs[1].Content != "reflow successful" {
		t.Fatalf("unexpected result msg: %+v", msgs[1])
	}
}

func TestTaskManager_LifecycleFail(t *testing.T) {
	mb, err := mailbox.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to initialize test mailbox: %v", err)
	}

	tm := NewTaskManager(mb)

	req := TaskDispatchRequest{
		TaskID:     "task-003",
		Objective:  "Flash firmware",
		Context:    "ESP32",
		TimeoutSec: 15,
	}

	_, err = tm.Dispatch(req, nil)
	if err != nil {
		t.Fatalf("failed to dispatch task: %v", err)
	}

	failErr := errors.New("bootloader sync failed")
	if err := tm.Fail("task-003", failErr); err != nil {
		t.Fatalf("failed to record task failure: %v", err)
	}

	task, ok := tm.Get("task-003")
	if !ok {
		t.Fatalf("task-003 not found")
	}
	if task.State != TaskStateFailed {
		t.Fatalf("expected state %v, got %v", TaskStateFailed, task.State)
	}
	if task.ErrorMsg != "bootloader sync failed" {
		t.Fatalf("expected error msg 'bootloader sync failed', got %q", task.ErrorMsg)
	}

	msgs, err := mb.Drain()
	if err != nil {
		t.Fatalf("failed to drain mailbox: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 mailbox messages, got %d", len(msgs))
	}
	if msgs[1].Kind != "task_error" || msgs[1].Content != "bootloader sync failed" {
		t.Fatalf("unexpected error message: %+v", msgs[1])
	}
}

func TestTaskManager_CancelAndTerminalRejection(t *testing.T) {
	mb, err := mailbox.New(t.TempDir())
	if err != nil {
		t.Fatalf("failed to initialize test mailbox: %v", err)
	}

	tm := NewTaskManager(mb)

	canceled := false
	cancelFn := context.CancelFunc(func() {
		canceled = true
	})

	req := TaskDispatchRequest{
		TaskID:     "task-004",
		Objective:  "Long running test",
		TimeoutSec: 100,
	}

	_, err = tm.Dispatch(req, cancelFn)
	if err != nil {
		t.Fatalf("failed to dispatch task: %v", err)
	}

	cancelRes, err := tm.Cancel("task-004", "aborted by operator")
	if err != nil {
		t.Fatalf("failed to cancel task: %v", err)
	}
	if !cancelRes.Canceled {
		t.Fatalf("expected cancel to succeed, got false")
	}
	if cancelRes.FinalState != "aborted" {
		t.Fatalf("expected final state 'aborted', got %q", cancelRes.FinalState)
	}
	if !canceled {
		t.Fatalf("expected cancelFn to have been invoked")
	}

	task, _ := tm.Get("task-004")
	if task.State != TaskStateAborted {
		t.Fatalf("expected state %v, got %v", TaskStateAborted, task.State)
	}

	cancelAgain, err := tm.Cancel("task-004", "duplicate cancel")
	if err != nil {
		t.Fatalf("unexpected error on second cancel: %v", err)
	}
	if cancelAgain.Canceled {
		t.Fatalf("expected second cancel to return canceled=false for terminal task")
	}
	if cancelAgain.FinalState != "aborted" {
		t.Fatalf("expected final state 'aborted', got %q", cancelAgain.FinalState)
	}
}

func TestTaskManager_ListActive(t *testing.T) {
	// Verify nil-mailbox safety
	tm := NewTaskManager(nil)

	_ = tm.UpdateProgress("non-existent", "test")

	_, _ = tm.Dispatch(TaskDispatchRequest{TaskID: "task-a", Objective: "Task A"}, nil)
	_, _ = tm.Dispatch(TaskDispatchRequest{TaskID: "task-b", Objective: "Task B"}, nil)
	_, _ = tm.Dispatch(TaskDispatchRequest{TaskID: "task-c", Objective: "Task C"}, nil)

	active := tm.ListActive()
	if len(active) != 3 {
		t.Fatalf("expected 3 active tasks, got %d", len(active))
	}

	_ = tm.Complete("task-b", "done", nil)
	_, _ = tm.Cancel("task-c", "stop")

	activeAfter := tm.ListActive()
	if len(activeAfter) != 1 {
		t.Fatalf("expected 1 active task, got %d", len(activeAfter))
	}
	if activeAfter[0].ID != "task-a" {
		t.Fatalf("expected active task to be 'task-a', got %q", activeAfter[0].ID)
	}
}

func TestTaskManager_ConcurrentAccess(t *testing.T) {
	tm := NewTaskManager(nil)
	_, _ = tm.Dispatch(TaskDispatchRequest{TaskID: "task-conc", Objective: "Stress test"}, nil)

	var wg sync.WaitGroup
	workers := 10
	iterations := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = tm.UpdateProgress("task-conc", "updating")
				_, _ = tm.Get("task-conc")
				_ = tm.ListActive()
			}
		}(i)
	}

	wg.Wait()

	task, ok := tm.Get("task-conc")
	if !ok || task == nil {
		t.Fatalf("expected task-conc to exist")
	}
}
