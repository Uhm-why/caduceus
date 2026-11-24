package auth

import (
	"errors"
	"testing"
)

func TestPolicyActionString(t *testing.T) {
	tests := []struct {
		name     string
		action   PolicyAction
		expected string
	}{
		{"unknown", PolicyUnknown, "unknown"},
		{"allow", PolicyAllow, "allow"},
		{"greylist", PolicyGreylist, "greylist"},
		{"deny", PolicyDeny, "deny"},
		{"out of bounds", PolicyAction(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.action.String(); got != tt.expected {
				t.Errorf("PolicyAction.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAuthStateString(t *testing.T) {
	tests := []struct {
		name     string
		state    AuthState
		expected string
	}{
		{"unknown", AuthStateUnknown, "unknown"},
		{"pending", AuthStatePending, "pending"},
		{"approved", AuthStateApproved, "approved"},
		{"denied", AuthStateDenied, "denied"},
		{"expired", AuthStateExpired, "expired"},
		{"out of bounds", AuthState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("AuthState.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestParsePolicyAction(t *testing.T) {
	tests := []struct {
		input       string
		expected    PolicyAction
		expectError bool
	}{
		{"allow", PolicyAllow, false},
		{"ALLOW", PolicyAllow, false},
		{"  greylist  ", PolicyGreylist, false},
		{"deny", PolicyDeny, false},
		{"unknown", PolicyUnknown, true},
		{"invalid", PolicyUnknown, true},
		{"", PolicyUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			action, err := ParsePolicyAction(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tt.input)
				}
				if !errors.Is(err, ErrInvalidPolicyAction) {
					t.Errorf("expected ErrInvalidPolicyAction, got %v", err)
				}
				if action != PolicyUnknown {
					t.Errorf("expected PolicyUnknown on error, got %v", action)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.input, err)
				}
				if action != tt.expected {
					t.Errorf("got %v, want %v", action, tt.expected)
				}
			}
		})
	}
}

func TestParseAuthState(t *testing.T) {
	tests := []struct {
		input       string
		expected    AuthState
		expectError bool
	}{
		{"pending", AuthStatePending, false},
		{"PENDING", AuthStatePending, false},
		{" approved ", AuthStateApproved, false},
		{"denied", AuthStateDenied, false},
		{"expired", AuthStateExpired, false},
		{"unknown", AuthStateUnknown, true},
		{"invalid", AuthStateUnknown, true},
		{"", AuthStateUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			state, err := ParseAuthState(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tt.input)
				}
				if !errors.Is(err, ErrInvalidAuthState) {
					t.Errorf("expected ErrInvalidAuthState, got %v", err)
				}
				if state != AuthStateUnknown {
					t.Errorf("expected AuthStateUnknown on error, got %v", state)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.input, err)
				}
				if state != tt.expected {
					t.Errorf("got %v, want %v", state, tt.expected)
				}
			}
		})
	}
}
