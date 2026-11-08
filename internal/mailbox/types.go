// Package mailbox provides a cross-platform, crash-safe message spool implementing
// the Maildir specification with atomic file rename delivery and fsync durability.
package mailbox

import (
	"time"
)

// Message represents a structured envelope stored in the Maildir spool or exchanged
// between peer agents.
type Message struct {
	ID            string    `json:"id,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	Kind          string    `json:"kind"`
	Timestamp     time.Time `json:"timestamp"`
	Sender        string    `json:"sender"`
	Content       string    `json:"content"`
}

// Mailbox defines the interface for durable message queuing and retrieval.
type Mailbox interface {
	// Enqueue persists a message into the spool.
	Enqueue(msg Message) error
	// Drain retrieves and removes all pending messages from the spool.
	Drain() ([]Message, error)
}
