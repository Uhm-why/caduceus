package mailbox

import (
	"time"
)

type Message struct {
	ID            string    `json:"id,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	Kind          string    `json:"kind,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Sender        string    `json:"sender"`
	Content       string    `json:"content"`
}

type Mailbox interface {
	Enqueue(msg Message) error
	Drain() ([]Message, error)
}
