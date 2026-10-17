package mailbox

import (
	"time"
)

type Message struct {
	Timestamp time.Time `json:"timestamp"`
	Sender    string    `json:"sender"`
	Content   string    `json:"content"`
}

type Mailbox interface {
	Enqueue(msg Message) error
	Drain() ([]Message, error)
}
