// Package auth provides policy-based authorization, human-in-the-loop greylist gating,
// and one-time password (OTP) verification for delegated task execution in Caduceus.
package auth

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	// ErrAuthNotFound is returned when a requested task authorization does not exist.
	ErrAuthNotFound = errors.New("task or auth request does not exist")
	// ErrAuthExpired is returned when the TTL deadline for an authorization has passed.
	ErrAuthExpired = errors.New("the TTL deadline for this authorization has passed")
	// ErrAuthInvalidToken is returned when a submitted OTP verification token does not match.
	ErrAuthInvalidToken = errors.New("supplied token does not match")
	// ErrAuthAlreadySettled is returned when attempting to settle or duplicate an authorization that is no longer pending.
	ErrAuthAlreadySettled = errors.New("the authorization request has already been settled")
	// ErrPolicyViolation is returned when an attempted action violates standing security policy.
	ErrPolicyViolation = errors.New("the attempted action is blacklisted")
	// ErrInvalidPolicyAction is returned when a policy action name cannot be parsed or resolves to PolicyUnknown.
	ErrInvalidPolicyAction = errors.New("invalid policy action")
	// ErrInvalidAuthState is returned when an authorization state name cannot be parsed or resolves to AuthStateUnknown.
	ErrInvalidAuthState = errors.New("invalid authorization state")
	// ErrInvalidPattern is returned when a policy rule contains malformed glob syntax.
	ErrInvalidPattern = errors.New("malformed glob syntax")
)

// PolicyAction represents the decision verdict for an evaluated action.
type PolicyAction int

var policyActionValue map[string]PolicyAction
var authStateValue map[string]AuthState

const (
	// PolicyUnknown represents an uninitialized or invalid policy decision.
	PolicyUnknown PolicyAction = iota
	// PolicyAllow immediately permits an action to execute without challenge.
	PolicyAllow
	// PolicyGreylist halts execution and requires human or elevated authorization.
	PolicyGreylist
	// PolicyDeny immediately blocks and rejects an action.
	PolicyDeny
)

// policyActionName maps a human readable name to a PolicyAction integer.
var policyActionName = map[PolicyAction]string{
	PolicyUnknown:  "unknown",
	PolicyAllow:    "allow",
	PolicyGreylist: "greylist",
	PolicyDeny:     "deny",
}

// String returns the string representation of a PolicyAction.
func (p PolicyAction) String() string {
	if name, ok := policyActionName[p]; ok {
		return name
	}

	return policyActionName[PolicyUnknown]
}

// AuthState represents the lifecycle status of an authorization request.
type AuthState int

const (
	// AuthStateUnknown represents an uninitialized or invalid authorization state.
	AuthStateUnknown AuthState = iota
	// AuthStatePending indicates the authorization request is awaiting approval or denial.
	AuthStatePending
	// AuthStateApproved indicates the authorization request was verified and approved.
	AuthStateApproved
	// AuthStateDenied indicates the authorization request was explicitly rejected.
	AuthStateDenied
	// AuthStateExpired indicates the authorization TTL deadline passed before settlement.
	AuthStateExpired
)

// authStateName maps a human readable word to the integer representing an Authorization State.
var authStateName = map[AuthState]string{
	AuthStateUnknown:  "unknown",
	AuthStatePending:  "pending",
	AuthStateApproved: "approved",
	AuthStateDenied:   "denied",
	AuthStateExpired:  "expired",
}

// String returns the string representation of an AuthState.
func (s AuthState) String() string {
	if name, ok := authStateName[s]; ok {
		return name
	}

	return authStateName[AuthStateUnknown]
}

func init() {
	policyActionValue = make(map[string]PolicyAction, len(policyActionName))
	authStateValue = make(map[string]AuthState, len(authStateName))

	for action, name := range policyActionName {
		policyActionValue[name] = action
	}

	for state, name := range authStateName {
		authStateValue[name] = state
	}
}

// ParsePolicyAction normalizes and converts a string into its corresponding PolicyAction.
// Returns ErrInvalidPolicyAction if the string does not match a valid action or is unknown.
func ParsePolicyAction(s string) (PolicyAction, error) {
	action, ok := policyActionValue[strings.ToLower(strings.TrimSpace(s))]

	if !ok {
		return PolicyUnknown, fmt.Errorf("%w: %q", ErrInvalidPolicyAction, s)
	}

	if action == PolicyUnknown {
		return PolicyUnknown, ErrInvalidPolicyAction
	}

	return action, nil
}

// ParseAuthState normalizes and converts a string into its corresponding AuthState.
// Returns ErrInvalidAuthState if the string does not match a valid state or is unknown.
func ParseAuthState(s string) (AuthState, error) {
	state, ok := authStateValue[strings.ToLower(strings.TrimSpace(s))]

	if !ok {
		return AuthStateUnknown, fmt.Errorf("%w: %q", ErrInvalidAuthState, s)
	}

	if state == AuthStateUnknown {
		return AuthStateUnknown, ErrInvalidAuthState
	}

	return state, nil
}

// PolicyRule defines a glob pattern matched against operation names, the decided PolicyAction,
// and an operator description explaining the rule's intent.
type PolicyRule struct {
	Pattern     string
	Action      PolicyAction
	Description string
}

// PendingAuth holds information about a task authorization request currently awaiting settlement.
type PendingAuth struct {
	TaskID    string
	PeerSAN   string
	Objective string
	Token     string
	State     AuthState
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Policy represents an evaluation ruleset and default fallback action guarded by a read-write mutex.
type Policy struct {
	mu            sync.RWMutex
	DefaultAction PolicyAction
	Rules         []PolicyRule
}

// AuthManager defines the interface for policy evaluation, OTP generation, and authorization lifecycle management.
type AuthManager interface {
	// Evaluate checks an action against policy rules.
	Evaluate(action string) PolicyAction
	// CreatePending generates an OTP and registers a pending authorization.
	CreatePending(taskID string, peerSAN string, objective string, ttl time.Duration) (*PendingAuth, error)
	// Verify tests the supplied token against the task's pending OTP.
	Verify(taskID string, token string) error
	// Deny explicitly marks a pending authorization as rejected.
	Deny(taskID string, reason string) error
	// Get retrieves a pending authorization by taskID.
	Get(taskID string) (*PendingAuth, bool)
	// ListPending returns all currently unexpired, pending authorizations.
	ListPending() []*PendingAuth
	// ExpireStale sweeps and marks expired authorizations.
	ExpireStale() int
}
