package rule

import (
	"errors"
	"fmt"

	"github.com/prabaltripathiofficial/edict/internal/event"
)

// validOperators is the set of comparison operators a condition may use.
var validOperators = map[Operator]bool{
	OpEq: true, OpNe: true, OpGt: true, OpGte: true, OpLt: true,
	OpLte: true, OpIn: true, OpContains: true, OpExists: true,
}

// validOperations is the set of change operations a rule may subscribe to.
var validOperations = map[event.Operation]bool{
	event.OpInsert: true, event.OpUpdate: true,
	event.OpReplace: true, event.OpDelete: true,
}

// Validate checks an entire rule set for structural and referential integrity.
// It validates each rule in isolation, enforces unique IDs, and verifies that
// every `then` reference points at a real rule and that no chain can exceed
// MaxChainedTriggers (which also rejects cycles).
func Validate(rules []Rule) error {
	byID := make(map[string]Rule, len(rules))
	for _, r := range rules {
		if _, dup := byID[r.ID]; dup {
			return fmt.Errorf("rule %q: duplicate rule id", r.ID)
		}
		if err := validateRule(r); err != nil {
			return err
		}
		byID[r.ID] = r
	}

	for _, r := range rules {
		for _, ref := range r.Then {
			if _, ok := byID[ref]; !ok {
				return fmt.Errorf("rule %q: then references unknown rule %q", r.ID, ref)
			}
		}
	}

	return validateChains(byID)
}

func validateRule(r Rule) error {
	if r.ID == "" {
		return errors.New("rule with empty id")
	}
	if r.Collection == "" {
		return fmt.Errorf("rule %q: collection is required", r.ID)
	}
	for _, op := range r.On {
		if !validOperations[op] {
			return fmt.Errorf("rule %q: invalid operation %q", r.ID, op)
		}
	}
	if len(r.Actions) == 0 {
		return fmt.Errorf("rule %q: at least one action is required", r.ID)
	}
	if r.When != nil {
		if err := validateMatch(r.ID, r.When); err != nil {
			return err
		}
	}
	for i, a := range r.Actions {
		if a.Type == "" {
			return fmt.Errorf("rule %q: action %d has empty type", r.ID, i)
		}
	}
	return nil
}

func validateMatch(ruleID string, m *Match) error {
	for _, c := range append(append([]Condition{}, m.All...), m.Any...) {
		if c.Field == "" {
			return fmt.Errorf("rule %q: condition with empty field", ruleID)
		}
		if !validOperators[c.Op] {
			return fmt.Errorf("rule %q: condition on %q has invalid op %q", ruleID, c.Field, c.Op)
		}
		if c.Op == OpExists {
			if _, ok := c.Value.(bool); !ok {
				return fmt.Errorf("rule %q: exists condition on %q requires a boolean value", ruleID, c.Field)
			}
		}
		if c.Op == OpIn {
			if _, ok := c.Value.([]any); !ok {
				return fmt.Errorf("rule %q: in condition on %q requires an array value", ruleID, c.Field)
			}
		}
	}
	return nil
}

// validateChains performs a depth-bounded walk from each rule following its
// `then` edges. Exceeding MaxChainedTriggers signals either an over-long chain
// or a cycle; both are rejected here so the engine never has to guard at
// runtime.
func validateChains(byID map[string]Rule) error {
	for id := range byID {
		if err := walkChain(byID, id, 0); err != nil {
			return err
		}
	}
	return nil
}

func walkChain(byID map[string]Rule, id string, depth int) error {
	if depth >= MaxChainedTriggers {
		return fmt.Errorf("rule %q: trigger chain exceeds limit of %d (possible cycle)", id, MaxChainedTriggers)
	}
	r := byID[id]
	for _, ref := range r.Then {
		if err := walkChain(byID, ref, depth+1); err != nil {
			return err
		}
	}
	return nil
}
