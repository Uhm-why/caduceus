package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// NewPolicy constructs a Policy with the specified default fallback action and initial rules.
func NewPolicy(defaultAction PolicyAction, rules []PolicyRule) *Policy {
	return &Policy{
		DefaultAction: defaultAction,
		Rules:         rules,
	}
}

// NewDefaultPolicy constructs a baseline Policy with PolicyGreylist as its default action
// and an empty rules slice, establishing a secure fail-safe posture.
func NewDefaultPolicy() *Policy {
	return &Policy{
		DefaultAction: PolicyGreylist,
		Rules:         []PolicyRule{},
	}
}

// Evaluate tests an action against the ordered policy rules and returns the first matching
// PolicyAction. If no rules match, it returns DefaultAction (or PolicyDeny if DefaultAction is uninitialized).
func (p *Policy) Evaluate(action string) PolicyAction {
	p.mu.RLock()
	defer p.mu.RUnlock()

	target := strings.TrimSpace(action)

	for _, rule := range p.Rules {
		matched, err := path.Match(rule.Pattern, target)
		if err != nil {
			return p.DefaultAction
		}

		if err == nil && matched {
			return rule.Action
		}

	}

	if p.DefaultAction == PolicyUnknown {
		return PolicyDeny
	}

	return p.DefaultAction
}

// AddRule validates and prepends a new rule to the policy, ensuring the newest operator
// decision takes precedence over broader wildcard rules. Returns ErrInvalidPolicyAction if rule.Action
// is PolicyUnknown, or ErrInvalidPattern if the pattern syntax is malformed.
func (p *Policy) AddRule(rule PolicyRule) error {
	if rule.Action == PolicyUnknown {
		return ErrInvalidPolicyAction
	}

	_, err := path.Match(rule.Pattern, "")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPattern, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.Rules = append([]PolicyRule{rule}, p.Rules...)

	return nil
}

// RemoveRule removes the first rule matching the given pattern.
// Returns true if a rule was removed, or false if not found.
func (p *Policy) RemoveRule(pattern string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, rule := range p.Rules {
		if rule.Pattern == pattern {
			p.Rules = append(p.Rules[:i], p.Rules[i+1:]...)
			return true
		}
	}

	return false
}

// RulesSnapshot returns an independent, deep-copied slice of current policy rules,
// safe for concurrent inspection or serialization without holding internal locks.
func (p *Policy) RulesSnapshot() []PolicyRule {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rules := make([]PolicyRule, len(p.Rules))
	_ = copy(rules, p.Rules)

	return rules
}

// LoadPolicy reads and parses a Policy from the JSON file at filePath. If the file does not
// exist or is empty, it returns NewDefaultPolicy() with a nil error so an unconfigured node
// can boot safely. Returns ErrInvalidPattern if any rule's glob pattern is malformed.
func LoadPolicy(filePath string) (*Policy, error) {
	policy, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewDefaultPolicy(), nil
		}
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	if len(strings.TrimSpace(string(policy))) == 0 {
		return NewDefaultPolicy(), nil
	}

	pf := &policyFile{}

	err = json.Unmarshal(policy, &pf)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling policyFile: %w", err)
	}

	if pf.DefaultAction == PolicyUnknown {
		pf.DefaultAction = PolicyGreylist
	}

	for _, rule := range pf.Rules {
		_, err = path.Match(rule.Pattern, "")
		if err != nil {
			return nil, fmt.Errorf("invalid policy rule pattern `%s`: %w", rule.Pattern, ErrInvalidPattern)
		}
	}

	return NewPolicy(pf.DefaultAction, pf.Rules), nil

}

// SavePolicy atomically persists p to the JSON file at filePath: it serializes a snapshot
// under a read lock, writes to a temporary file in the same directory, fsyncs for durability,
// restricts permissions to 0600 on POSIX systems, and renames over the destination. Returns
// an error if p is nil.
func SavePolicy(filePath string, p *Policy) error {
	if p == nil {
		return errors.New("policy is nil")
	}

	dir := filepath.Dir(filePath)

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	pf := policyFile{
		DefaultAction: p.DefaultAction,
		Rules:         p.Rules,
	}

	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling policy file: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "policy-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmp.Name(), 0600)
	}

	if err := os.Rename(tmp.Name(), filePath); err != nil {
		return err
	}

	return nil
}
