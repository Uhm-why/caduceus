package auth

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestNewDefaultPolicy(t *testing.T) {
	p := NewDefaultPolicy()
	if p.DefaultAction != PolicyGreylist {
		t.Errorf("expected default action PolicyGreylist, got %v", p.DefaultAction)
	}
	if len(p.Rules) != 0 {
		t.Errorf("expected empty initial rules, got %d", len(p.Rules))
	}
}

func TestNewPolicy(t *testing.T) {
	rules := []PolicyRule{
		{Pattern: "terminal.*", Action: PolicyDeny, Description: "block shell"},
	}
	p := NewPolicy(PolicyAllow, rules)
	if p.DefaultAction != PolicyAllow {
		t.Errorf("expected default action PolicyAllow, got %v", p.DefaultAction)
	}
	if len(p.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(p.Rules))
	}
}

func TestPolicyEvaluate(t *testing.T) {
	tests := []struct {
		name          string
		defaultAction PolicyAction
		rules         []PolicyRule
		action        string
		expected      PolicyAction
	}{
		{
			name:          "exact match allow",
			defaultAction: PolicyDeny,
			rules: []PolicyRule{
				{Pattern: "mail.read", Action: PolicyAllow},
			},
			action:   "mail.read",
			expected: PolicyAllow,
		},
		{
			name:          "glob match greylist",
			defaultAction: PolicyDeny,
			rules: []PolicyRule{
				{Pattern: "terminal.*", Action: PolicyGreylist},
			},
			action:   "terminal.exec",
			expected: PolicyGreylist,
		},
		{
			name:          "catch-all match",
			defaultAction: PolicyGreylist,
			rules: []PolicyRule{
				{Pattern: "*", Action: PolicyDeny},
			},
			action:   "custom.action",
			expected: PolicyDeny,
		},
		{
			name:          "whitespace trimmed",
			defaultAction: PolicyDeny,
			rules: []PolicyRule{
				{Pattern: "system.info", Action: PolicyAllow},
			},
			action:   "  system.info  ",
			expected: PolicyAllow,
		},
		{
			name:          "unmatched falls back to default action",
			defaultAction: PolicyGreylist,
			rules: []PolicyRule{
				{Pattern: "mail.*", Action: PolicyAllow},
			},
			action:   "terminal.exec",
			expected: PolicyGreylist,
		},
		{
			name:          "unmatched with PolicyUnknown falls back to PolicyDeny",
			defaultAction: PolicyUnknown,
			rules: []PolicyRule{
				{Pattern: "mail.*", Action: PolicyAllow},
			},
			action:   "terminal.exec",
			expected: PolicyDeny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPolicy(tt.defaultAction, tt.rules)
			got := p.Evaluate(tt.action)
			if got != tt.expected {
				t.Errorf("Evaluate(%q) = %v, want %v", tt.action, got, tt.expected)
			}
		})
	}
}

func TestPolicyOverridePrecedence(t *testing.T) {
	p := NewDefaultPolicy()

	// Base rule: greylist all terminal commands
	err := p.AddRule(PolicyRule{
		Pattern:     "terminal.*",
		Action:      PolicyGreylist,
		Description: "greylist shell execution",
	})
	if err != nil {
		t.Fatalf("unexpected error adding base rule: %v", err)
	}

	// terminal.exec should be greylisted
	if got := p.Evaluate("terminal.exec"); got != PolicyGreylist {
		t.Errorf("expected PolicyGreylist, got %v", got)
	}

	// Operator override: Always Allow terminal.status
	err = p.AddRule(PolicyRule{
		Pattern:     "terminal.status",
		Action:      PolicyAllow,
		Description: "operator override: allow status checks",
	})
	if err != nil {
		t.Fatalf("unexpected error adding override rule: %v", err)
	}

	// terminal.status should now be immediately allowed via prepended rule
	if got := p.Evaluate("terminal.status"); got != PolicyAllow {
		t.Errorf("expected PolicyAllow for overridden action, got %v", got)
	}

	// Other terminal commands should remain greylisted
	if got := p.Evaluate("terminal.exec"); got != PolicyGreylist {
		t.Errorf("expected PolicyGreylist for non-overridden action, got %v", got)
	}

	// Operator override: Always Deny terminal.reboot
	err = p.AddRule(PolicyRule{
		Pattern:     "terminal.reboot",
		Action:      PolicyDeny,
		Description: "operator override: deny reboot",
	})
	if err != nil {
		t.Fatalf("unexpected error adding deny override: %v", err)
	}

	if got := p.Evaluate("terminal.reboot"); got != PolicyDeny {
		t.Errorf("expected PolicyDeny, got %v", got)
	}
}

func TestAddRuleValidation(t *testing.T) {
	p := NewDefaultPolicy()

	// Invalid action (PolicyUnknown)
	err := p.AddRule(PolicyRule{
		Pattern: "terminal.*",
		Action:  PolicyUnknown,
	})
	if !errors.Is(err, ErrInvalidPolicyAction) {
		t.Errorf("expected ErrInvalidPolicyAction, got %v", err)
	}

	// Malformed glob pattern
	err = p.AddRule(PolicyRule{
		Pattern: "terminal.[a-",
		Action:  PolicyAllow,
	})
	if !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("expected ErrInvalidPattern, got %v", err)
	}
}

func TestRulesSnapshot(t *testing.T) {
	p := NewDefaultPolicy()
	_ = p.AddRule(PolicyRule{Pattern: "action1", Action: PolicyAllow})
	_ = p.AddRule(PolicyRule{Pattern: "action2", Action: PolicyDeny})

	snap := p.RulesSnapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 rules in snapshot, got %d", len(snap))
	}

	// Mutating the returned slice should not alter the policy's internal slice
	snap[0].Pattern = "mutated"
	freshSnap := p.RulesSnapshot()
	if freshSnap[0].Pattern == "mutated" {
		t.Errorf("modifying snapshot mutated internal policy rules")
	}

	// Adding another rule should not alter the previous snapshot
	_ = p.AddRule(PolicyRule{Pattern: "action3", Action: PolicyGreylist})
	if len(snap) != 2 {
		t.Errorf("previous snapshot mutated after AddRule; len = %d", len(snap))
	}
}

func TestPolicyConcurrency(t *testing.T) {
	p := NewDefaultPolicy()
	var wg sync.WaitGroup

	// Concurrently add rules and evaluate
	for i := 0; i < 50; i++ {
		wg.Add(2)
		idx := i

		// Writer
		go func() {
			defer wg.Done()
			_ = p.AddRule(PolicyRule{
				Pattern: fmt.Sprintf("action.%d", idx),
				Action:  PolicyAllow,
			})
		}()

		// Reader
		go func() {
			defer wg.Done()
			_ = p.Evaluate(fmt.Sprintf("action.%d", idx))
			_ = p.RulesSnapshot()
		}()
	}

	wg.Wait()
}
