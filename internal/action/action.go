// Package action defines the side effects an Edict rule can trigger and the
// dispatcher that runs them with at-least-once delivery semantics. Action types
// (log, webhook, ...) are built from a rule.ActionSpec by a registry, so adding
// a new action type is a matter of registering one builder.
package action

import (
	"context"
	"fmt"

	"github.com/prabaltripathiofficial/edict/internal/event"
	"github.com/prabaltripathiofficial/edict/internal/rule"
)

// Action is a single executable side effect. Implementations must be safe to
// call repeatedly: the dispatcher may retry on failure, so an Action should aim
// to be idempotent or tolerate at-least-once delivery.
type Action interface {
	// Type is the spec type this action was built from (used in audit records).
	Type() string
	// Execute performs the side effect for one fired rule/event pair.
	Execute(ctx context.Context, ev event.ChangeEvent, r rule.Rule) error
}

// Builder constructs an Action from its declarative spec, validating any
// type-specific fields. It is called once per rule action at dispatcher
// construction time, not per event.
type Builder func(spec rule.ActionSpec) (Action, error)

// registry maps an action type to its builder.
var registry = map[string]Builder{}

// Register installs a builder for an action type. It is intended to be called
// from package init functions of the built-in actions.
func Register(actionType string, b Builder) {
	registry[actionType] = b
}

// Build resolves and constructs the Action for a spec.
func Build(spec rule.ActionSpec) (Action, error) {
	b, ok := registry[spec.Type]
	if !ok {
		return nil, fmt.Errorf("unknown action type %q", spec.Type)
	}
	return b(spec)
}
