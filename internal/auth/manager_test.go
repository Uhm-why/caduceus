package auth

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

var _ AuthManager = (*DefaultAuthManager)(nil)

func TestNewAuthManagerDefaults(t *testing.T) {
	mgr := NewAuthManager(nil, 0)
	if mgr.policy == nil {
		t.Fatalf("expected non-nil default policy")
	}
	if mgr.defaultTTL != 5*time.Minute {
		t.Errorf("expected default TTL 5m, got %v", mgr.defaultTTL)
	}
	if mgr.pending == nil {
		t.Fatalf("expected initialized pending map")
	}
}

func TestAuthManagerEvaluate(t *testing.T) {
	rules := []PolicyRule{
		{Pattern: "terminal.exec", Action: PolicyDeny},
	}
	policy := NewPolicy(PolicyAllow, rules)
	mgr := NewAuthManager(policy, time.Minute)

	if got := mgr.Evaluate("terminal.exec"); got != PolicyDeny {
		t.Errorf("expected PolicyDeny, got %v", got)
	}
	if got := mgr.Evaluate("mailbox.read"); got != PolicyAllow {
		t.Errorf("expected PolicyAllow, got %v", got)
	}
}

func TestCreatePendingValidation(t *testing.T) {
	mgr := NewAuthManager(nil, time.Minute)

	// Missing taskID
	_, err := mgr.CreatePending("", "peer1", "execute script", time.Minute)
	if err == nil {
		t.Errorf("expected error for empty taskID, got nil")
	}

	// Missing peerSAN
	_, err = mgr.CreatePending("task-1", "", "execute script", time.Minute)
	if err == nil {
		t.Errorf("expected error for empty peerSAN, got nil")
	}

	// Valid creation
	auth, err := mgr.CreatePending("task-1", "node-a.caduceus.local", "exec", 0)
	if err != nil {
		t.Fatalf("unexpected error creating pending auth: %v", err)
	}

	if auth.TaskID != "task-1" {
		t.Errorf("expected task-1, got %q", auth.TaskID)
	}
	if auth.PeerSAN != "node-a.caduceus.local" {
		t.Errorf("expected node-a.caduceus.local, got %q", auth.PeerSAN)
	}
	if auth.State != AuthStatePending {
		t.Errorf("expected AuthStatePending, got %v", auth.State)
	}
	if len(auth.Token) != 8 {
		t.Errorf("expected 8-char hex token, got %q", auth.Token)
	}
	if auth.ExpiresAt.Before(auth.CreatedAt) {
		t.Errorf("expiresAt before createdAt")
	}
}

func TestCreatePendingDuplicateGuard(t *testing.T) {
	mgr := NewAuthManager(nil, time.Minute)

	_, err := mgr.CreatePending("task-1", "peer1", "run", time.Minute)
	if err != nil {
		t.Fatalf("first creation failed: %v", err)
	}

	// Duplicate active pending task must fail
	_, err = mgr.CreatePending("task-1", "peer1", "run", time.Minute)
	if !errors.Is(err, ErrAuthAlreadySettled) {
		t.Errorf("expected ErrAuthAlreadySettled on active duplicate, got %v", err)
	}

	// Settle the task
	auth, _ := mgr.Get("task-1")
	if err := mgr.Verify("task-1", auth.Token); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// Settled (approved) task must reject new pending auth
	_, err = mgr.CreatePending("task-1", "peer1", "run", time.Minute)
	if !errors.Is(err, ErrAuthAlreadySettled) {
		t.Errorf("expected ErrAuthAlreadySettled on approved task, got %v", err)
	}
}

func TestVerifyLifecycle(t *testing.T) {
	mgr := NewAuthManager(nil, time.Minute)

	// Verify non-existent task
	err := mgr.Verify("missing", "anytoken")
	if !errors.Is(err, ErrAuthNotFound) {
		t.Errorf("expected ErrAuthNotFound, got %v", err)
	}

	auth, err := mgr.CreatePending("task-1", "peer1", "exec", time.Minute)
	if err != nil {
		t.Fatalf("failed to create pending: %v", err)
	}

	// Invalid token
	err = mgr.Verify("task-1", "wrongtoken")
	if !errors.Is(err, ErrAuthInvalidToken) {
		t.Errorf("expected ErrAuthInvalidToken, got %v", err)
	}

	// Correct token
	err = mgr.Verify("task-1", auth.Token)
	if err != nil {
		t.Fatalf("unexpected error verifying valid token: %v", err)
	}

	// Check state updated to Approved
	settled, ok := mgr.Get("task-1")
	if !ok || settled.State != AuthStateApproved {
		t.Errorf("expected task to be AuthStateApproved, got state %v", settled.State)
	}

	// Already settled verification must fail
	err = mgr.Verify("task-1", auth.Token)
	if !errors.Is(err, ErrAuthAlreadySettled) {
		t.Errorf("expected ErrAuthAlreadySettled on second verify, got %v", err)
	}
}

func TestVerifyLockout(t *testing.T) {
	mgr := NewAuthManager(nil, time.Minute)
	auth, err := mgr.CreatePending("task-lockout", "peer1", "exec", time.Minute)
	if err != nil {
		t.Fatalf("failed to create pending: %v", err)
	}

	// Strike 1
	_ = mgr.Verify("task-lockout", "wrong-1")
	cur, _ := mgr.Get("task-lockout")
	if cur.State != AuthStatePending || cur.FailedAttempts != 1 {
		t.Errorf("expected 1 failed attempt, pending state; got %d, %v", cur.FailedAttempts, cur.State)
	}

	// Strike 2
	_ = mgr.Verify("task-lockout", "wrong-2")
	cur, _ = mgr.Get("task-lockout")
	if cur.State != AuthStatePending || cur.FailedAttempts != 2 {
		t.Errorf("expected 2 failed attempts, pending state; got %d, %v", cur.FailedAttempts, cur.State)
	}

	// Strike 3 - Lockout!
	_ = mgr.Verify("task-lockout", "wrong-3")
	cur, _ = mgr.Get("task-lockout")
	if cur.State != AuthStateDenied || cur.FailedAttempts != 3 {
		t.Errorf("expected 3 failed attempts and AuthStateDenied lockout; got %d, %v", cur.FailedAttempts, cur.State)
	}

	// Even correct token must be rejected now because task is locked out / settled
	err = mgr.Verify("task-lockout", auth.Token)
	if !errors.Is(err, ErrAuthAlreadySettled) {
		t.Errorf("expected ErrAuthAlreadySettled on locked-out task, got %v", err)
	}
}

func TestVerifyExpiration(t *testing.T) {
	mgr := NewAuthManager(nil, time.Millisecond)

	// Create with 1ms TTL
	auth, err := mgr.CreatePending("task-expired", "peer1", "exec", time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create pending: %v", err)
	}

	// Wait for expiration
	time.Sleep(5 * time.Millisecond)

	err = mgr.Verify("task-expired", auth.Token)
	if !errors.Is(err, ErrAuthExpired) {
		t.Errorf("expected ErrAuthExpired, got %v", err)
	}

	// Verify state transitioned to Expired
	settled, ok := mgr.Get("task-expired")
	if !ok || settled.State != AuthStateExpired {
		t.Errorf("expected AuthStateExpired, got %v", settled.State)
	}
}

func TestDenyLifecycle(t *testing.T) {
	mgr := NewAuthManager(nil, time.Minute)

	// Deny non-existent task
	if err := mgr.Deny("missing", "reason"); !errors.Is(err, ErrAuthNotFound) {
		t.Errorf("expected ErrAuthNotFound, got %v", err)
	}

	_, err := mgr.CreatePending("task-1", "peer1", "exec", time.Minute)
	if err != nil {
		t.Fatalf("failed to create pending: %v", err)
	}

	// Deny pending task
	if err := mgr.Deny("task-1", "operator rejected"); err != nil {
		t.Fatalf("unexpected error denying task: %v", err)
	}

	denied, ok := mgr.Get("task-1")
	if !ok || denied.State != AuthStateDenied {
		t.Errorf("expected AuthStateDenied, got %v", denied.State)
	}

	// Denying already settled task
	if err := mgr.Deny("task-1", "again"); !errors.Is(err, ErrAuthAlreadySettled) {
		t.Errorf("expected ErrAuthAlreadySettled, got %v", err)
	}
}

func TestGetCloningImmutability(t *testing.T) {
	mgr := NewAuthManager(nil, time.Minute)

	auth, err := mgr.CreatePending("task-1", "peer1", "exec", time.Minute)
	if err != nil {
		t.Fatalf("failed to create pending: %v", err)
	}

	got, ok := mgr.Get("task-1")
	if !ok {
		t.Fatalf("expected task to be found")
	}

	// Mutate the returned copy
	got.State = AuthStateApproved
	got.Objective = "mutated"

	// Fetch again from manager and assert internal state was not changed
	fresh, ok := mgr.Get("task-1")
	if !ok {
		t.Fatalf("expected task to be found")
	}
	if fresh.State != AuthStatePending {
		t.Errorf("internal state was mutated by caller: %v", fresh.State)
	}
	if fresh.Objective == "mutated" {
		t.Errorf("internal objective was mutated by caller")
	}
	if fresh.Token != auth.Token {
		t.Errorf("token mismatch")
	}
}

func TestListPending(t *testing.T) {
	mgr := NewAuthManager(nil, time.Minute)

	// Task 1: active pending
	_, _ = mgr.CreatePending("task-1", "peer1", "exec1", time.Minute)

	// Task 2: approved
	t2, _ := mgr.CreatePending("task-2", "peer2", "exec2", time.Minute)
	_ = mgr.Verify("task-2", t2.Token)

	// Task 3: denied
	_, _ = mgr.CreatePending("task-3", "peer3", "exec3", time.Minute)
	_ = mgr.Deny("task-3", "unauthorized")

	// Task 4: active pending
	_, _ = mgr.CreatePending("task-4", "peer4", "exec4", time.Minute)

	// Task 5: already expired
	_, _ = mgr.CreatePending("task-5", "peer5", "exec5", time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	pending := mgr.ListPending()
	if len(pending) != 2 {
		t.Fatalf("expected 2 active pending tasks, got %d", len(pending))
	}

	taskIDs := map[string]bool{pending[0].TaskID: true, pending[1].TaskID: true}
	if !taskIDs["task-1"] || !taskIDs["task-4"] {
		t.Errorf("expected task-1 and task-4 in pending list, got %v", taskIDs)
	}
}

func TestExpireStale(t *testing.T) {
	mgr := NewAuthManager(nil, time.Minute)

	// 2 unexpired tasks
	_, _ = mgr.CreatePending("fresh-1", "peer1", "exec", time.Minute)
	_, _ = mgr.CreatePending("fresh-2", "peer2", "exec", time.Minute)

	// 2 expired tasks
	_, _ = mgr.CreatePending("stale-1", "peer1", "exec", time.Millisecond)
	_, _ = mgr.CreatePending("stale-2", "peer2", "exec", time.Millisecond)

	time.Sleep(5 * time.Millisecond)

	expiredCount := mgr.ExpireStale()
	if expiredCount != 2 {
		t.Errorf("expected 2 expired tasks swept, got %d", expiredCount)
	}

	stale1, ok := mgr.Get("stale-1")
	if !ok || stale1.State != AuthStateExpired {
		t.Errorf("expected stale-1 to be marked AuthStateExpired, got %v", stale1.State)
	}

	fresh1, ok := mgr.Get("fresh-1")
	if !ok || fresh1.State != AuthStatePending {
		t.Errorf("expected fresh-1 to remain AuthStatePending, got %v", fresh1.State)
	}
}

func TestAuthManagerConcurrency(t *testing.T) {
	mgr := NewAuthManager(nil, 50*time.Millisecond)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(3)
		taskID := fmt.Sprintf("task-conc-%d", i)

		// Goroutine 1: Create and Verify
		go func(id string) {
			defer wg.Done()
			auth, err := mgr.CreatePending(id, "peer.caduceus.local", "action", 50*time.Millisecond)
			if err == nil && auth != nil {
				_ = mgr.Verify(id, auth.Token)
			}
		}(taskID)

		// Goroutine 2: Readers (Get, ListPending, Evaluate)
		go func(id string) {
			defer wg.Done()
			_, _ = mgr.Get(id)
			_ = mgr.ListPending()
			_ = mgr.Evaluate("terminal.exec")
		}(taskID)

		// Goroutine 3: Sweeper / Denier
		go func(id string) {
			defer wg.Done()
			_ = mgr.ExpireStale()
			_ = mgr.Deny(id, "concurrent test reject")
		}(taskID)
	}

	wg.Wait()
}
