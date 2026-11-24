package auth

import (
	"fmt"
	"path"
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

// RulesSnapshot returns an independent, deep-copied slice of current policy rules,
// safe for concurrent inspection or serialization without holding internal locks.
func (p *Policy) RulesSnapshot() []PolicyRule {
	p.mu.RLock()
	defer p.mu.RUnlock()

	rules := make([]PolicyRule, len(p.Rules))
	_ = copy(rules, p.Rules)

	return rules
}
