package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// DefaultAuthManager implements the AuthManager interface, providing thread-safe
// in-memory authorization lifecycle management and constant-time OTP verification.
type DefaultAuthManager struct {
	mu         sync.RWMutex
	policy     *Policy
	pending    map[string]*PendingAuth
	defaultTTL time.Duration
}

// generateOTP reads 4 bytes of cryptographically secure random entropy and returns
// an 8-character hexadecimal token string.
func generateOTP() (string, error) {
	bytes := make([]byte, 4)

	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil
}

// NewAuthManager constructs a DefaultAuthManager with the provided Policy and default TTL.
// If policy is nil, it falls back to NewDefaultPolicy. If defaultTTL is non-positive,
// it defaults to 5 minutes.
func NewAuthManager(policy *Policy, defaultTTL time.Duration) *DefaultAuthManager {
	if policy == nil {
		policy = NewDefaultPolicy()
	}

	if defaultTTL <= 0 {
		defaultTTL = 5 * time.Minute
	}

	initPending := make(map[string]*PendingAuth)
	return &DefaultAuthManager{
		policy:     policy,
		defaultTTL: defaultTTL,
		pending:    initPending,
	}
}

// Evaluate checks an action against the underlying policy rules and returns the decided PolicyAction.
func (m *DefaultAuthManager) Evaluate(action string) PolicyAction {
	return m.policy.Evaluate(action)
}

// CreatePending generates a cryptographically random OTP token and registers a pending
// authorization record for taskID. Returns ErrAuthAlreadySettled if the task is actively pending
// or was already approved/denied.
func (m *DefaultAuthManager) CreatePending(taskID string, peerSAN string, objective string, ttl time.Duration) (*PendingAuth, error) {

	if taskID == "" {
		return nil, fmt.Errorf("Task ID is required to create a pending auth.")
	}

	if peerSAN == "" {
		return nil, fmt.Errorf("Peer SAN is required to create a pending auth.")
	}

	if ttl <= 0 {
		ttl = m.defaultTTL
	}

	now := time.Now()

	token, err := generateOTP()
	if err != nil {
		return nil, err
	}

	auth := &PendingAuth{
		TaskID:    taskID,
		PeerSAN:   peerSAN,
		Objective: objective,
		Token:     token,
		State:     AuthStatePending,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, exists := m.pending[taskID]; exists {
		if existing.State == AuthStatePending && now.Before(existing.ExpiresAt) {
			return nil, ErrAuthAlreadySettled
		}

		if existing.State == AuthStateApproved || existing.State == AuthStateDenied {
			return nil, ErrAuthAlreadySettled
		}
	}

	m.pending[taskID] = auth
	return auth, nil
}

// Verify validates the supplied OTP token against the pending authorization for taskID using
// constant-time comparison. Transitions the state to AuthStateApproved on success, or
// AuthStateExpired if the TTL has elapsed.
func (m *DefaultAuthManager) Verify(taskID string, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	auth, exists := m.pending[taskID]
	if !exists {
		return ErrAuthNotFound
	}

	if auth.State != AuthStatePending {
		return ErrAuthAlreadySettled
	}

	if time.Now().After(auth.ExpiresAt) {
		auth.State = AuthStateExpired
		return ErrAuthExpired
	}

	if subtle.ConstantTimeCompare([]byte(auth.Token), []byte(token)) == 0 {
		return ErrAuthInvalidToken
	}

	auth.State = AuthStateApproved
	return nil
}

// Deny explicitly transitions a pending authorization to AuthStateDenied. Returns
// ErrAuthNotFound if taskID does not exist, or ErrAuthAlreadySettled if not pending.
func (m *DefaultAuthManager) Deny(taskID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	auth, exists := m.pending[taskID]
	if !exists {
		return ErrAuthNotFound
	}

	if auth.State != AuthStatePending {
		return ErrAuthAlreadySettled
	}

	auth.State = AuthStateDenied

	return nil
}

// Get returns an independent, thread-safe clone of the pending authorization for taskID,
// or false if the task is not found.
func (m *DefaultAuthManager) Get(taskID string) (*PendingAuth, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	auth, exists := m.pending[taskID]
	if !exists {
		return nil, false
	}

	cp := *auth
	return &cp, true
}

// ListPending returns independent, thread-safe clones of all authorizations that are currently
// in AuthStatePending and have not expired.
func (m *DefaultAuthManager) ListPending() []*PendingAuth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := []*PendingAuth{}

	for _, auth := range m.pending {
		if auth.State == AuthStatePending && time.Now().Before(auth.ExpiresAt) {
			cp := *auth
			list = append(list, &cp)
		}
	}

	return list
}

// ExpireStale sweeps all pending authorizations past their TTL deadline, marks them
// as AuthStateExpired, and returns the total number of swept records.
func (m *DefaultAuthManager) ExpireStale() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	counter := 0
	for _, auth := range m.pending {
		if auth.State == AuthStatePending && time.Now().After(auth.ExpiresAt) {
			auth.State = AuthStateExpired
			counter++
		}
	}

	return counter
}
