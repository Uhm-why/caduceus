package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestLoadPolicy_Defaults(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent file should return NewDefaultPolicy
	missingPath := filepath.Join(tmpDir, "missing.json")
	p, err := LoadPolicy(missingPath)
	if err != nil {
		t.Fatalf("unexpected error for missing policy file: %v", err)
	}
	if p.DefaultAction != PolicyGreylist {
		t.Errorf("expected default action PolicyGreylist, got %v", p.DefaultAction)
	}
	if len(p.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(p.Rules))
	}

	// Empty file should return NewDefaultPolicy
	emptyPath := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(emptyPath, []byte("   \n  "), 0600); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}
	pEmpty, err := LoadPolicy(emptyPath)
	if err != nil {
		t.Fatalf("unexpected error for empty policy file: %v", err)
	}
	if pEmpty.DefaultAction != PolicyGreylist {
		t.Errorf("expected default action PolicyGreylist, got %v", pEmpty.DefaultAction)
	}
}

func TestSaveAndLoadPolicy_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.json")

	// Nil policy save should fail
	if err := SavePolicy(policyPath, nil); err == nil {
		t.Errorf("expected error saving nil policy, got nil")
	}

	// Create and populate policy
	p := NewPolicy(PolicyDeny, []PolicyRule{
		{Pattern: "terminal.status", Action: PolicyAllow, Description: "allow status"},
		{Pattern: "terminal.*", Action: PolicyGreylist, Description: "greylist shell"},
	})

	if err := SavePolicy(policyPath, p); err != nil {
		t.Fatalf("unexpected error saving policy: %v", err)
	}

	// Verify file permissions on POSIX
	if runtime.GOOS != "windows" {
		info, err := os.Stat(policyPath)
		if err != nil {
			t.Fatalf("failed to stat saved policy: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("expected file mode 0600, got %o", info.Mode().Perm())
		}
	}

	// Verify content on disk uses human-readable actions ("allow", "greylist", "deny")
	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("failed to read saved policy: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"allow"`) || !strings.Contains(content, `"greylist"`) {
		t.Errorf("expected human-readable actions in JSON, got:\n%s", content)
	}

	// Load back
	loaded, err := LoadPolicy(policyPath)
	if err != nil {
		t.Fatalf("unexpected error loading saved policy: %v", err)
	}

	if loaded.DefaultAction != PolicyDeny {
		t.Errorf("expected DefaultAction PolicyDeny, got %v", loaded.DefaultAction)
	}
	if len(loaded.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(loaded.Rules))
	}
	if loaded.Rules[0].Pattern != "terminal.status" || loaded.Rules[0].Action != PolicyAllow {
		t.Errorf("rule 0 mismatch: %+v", loaded.Rules[0])
	}
	if loaded.Rules[1].Pattern != "terminal.*" || loaded.Rules[1].Action != PolicyGreylist {
		t.Errorf("rule 1 mismatch: %+v", loaded.Rules[1])
	}
}

func TestLoadPolicy_ValidationErrors(t *testing.T) {
	tmpDir := t.TempDir()

	// Malformed JSON
	badJSONPath := filepath.Join(tmpDir, "bad.json")
	_ = os.WriteFile(badJSONPath, []byte(`{invalid json`), 0600)
	if _, err := LoadPolicy(badJSONPath); err == nil {
		t.Errorf("expected error for malformed json, got nil")
	}

	// Malformed glob pattern in rule
	badPatternPath := filepath.Join(tmpDir, "bad_pattern.json")
	badPatternData := `{
		"default_action": "deny",
		"rules": [{"pattern": "terminal.[a-", "action": "allow"}]
	}`
	_ = os.WriteFile(badPatternPath, []byte(badPatternData), 0600)
	_, err := LoadPolicy(badPatternPath)
	if err == nil {
		t.Fatalf("expected error for invalid glob pattern, got nil")
	}
	if !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("expected ErrInvalidPattern, got %v", err)
	}
}
