package peer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type TaskState int

const (
	TaskStateUnknown TaskState = iota
	TaskStateQueued
	TaskStateRunning
	TaskStateCompleted
	TaskStateFailed
	TaskStateAborted
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

func (ts TaskState) MarshalJSON() ([]byte, error) {
	marshaled, err := json.Marshal(ts.String())
	return marshaled, err
}

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

func (ts TaskState) String() string {
	if name, ok := taskStateName[ts]; ok {
		return name
	}

	return taskStateName[TaskStateUnknown]
}

type TaskDispatchRequest struct {
	TaskID     string `json:"task_id"`
	Objective  string `json:"objective"`
	Context    string `json:"context"`
	TimeoutSec int    `json:"timeout_sec"`
	Priority   int    `json:"priority,omitempty"`
}

type TaskDispatchResponse struct {
	TaskID       string `json:"task_id"`
	Accepted     bool   `json:"accepted"`
	Reason       string `json:"reason,omitempty"`
	InitialState string `json:"initial_state,omitempty"`
}

type TaskStatusRequest struct {
	TaskID string `json:"task_id"`
}

type TaskStatusResponse struct {
	TaskID     string   `json:"task_id"`
	State      string   `json:"state"`
	Progress   string   `json:"progress"`
	Result     string   `json:"result"`
	Artifacts  []string `json:"artifacts,omitempty"`
	Error      string   `json:"error,omitempty"`
	ElapsedSec int64    `json:"elapsed_sec"`
}

type TaskCancelRequest struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type TaskCancelResponse struct {
	TaskID     string `json:"task_id"`
	Canceled   bool   `json:"canceled"`
	FinalState string `json:"final_state"`
}

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
