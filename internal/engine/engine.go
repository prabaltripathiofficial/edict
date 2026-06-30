// Package engine evaluates change events against a rule set and dispatches the
// actions of every matching rule, following `then` chains to fire downstream
// rules. It is deliberately decoupled from both the event source and the action
// implementations: it consumes event.ChangeEvent and delegates side effects to
// a Dispatcher.
package engine

import (
	"context"
	"log/slog"

	"github.com/prabaltripathiofficial/edict/internal/event"
	"github.com/prabaltripathiofficial/edict/internal/rule"
)

// Dispatcher runs the actions of a fired rule. The engine never executes side
// effects itself; it hands matched rules to a Dispatcher so retry, delivery
// semantics, and action types stay in one place (see the action package).
type Dispatcher interface {
	Dispatch(ctx context.Context, r rule.Rule, ev event.ChangeEvent) error
}

// Engine matches events and drives action dispatch.
type Engine struct {
	rules      *rule.Set
	dispatcher Dispatcher
	log        *slog.Logger
}

// New builds an Engine over a rule set and dispatcher.
func New(rules *rule.Set, d Dispatcher, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	return &Engine{rules: rules, dispatcher: d, log: log}
}

// HandleEvent evaluates ev against the rules registered for its collection and
// fires every match. It is safe to call once per change event.
//
// Returning an error is reserved for context cancellation; per-rule dispatch
// failures are logged and do not abort the remaining rules, because one
// unreachable webhook should not silence every other trigger on the event.
func (e *Engine) HandleEvent(ctx context.Context, ev event.ChangeEvent) error {
	// fired guards against firing the same rule twice for one originating event,
	// which both bounds work and breaks diamond shapes in the rule graph (two
	// rules chaining into a third). The chain length itself is bounded at load
	// time by rule.Validate.
	fired := make(map[string]bool)

	for _, r := range e.rules.ForCollection(ev.Collection) {
		if err := e.evaluate(ctx, r, ev, fired); err != nil {
			return err
		}
	}
	return nil
}

// evaluate checks one rule against the event and, if it matches, dispatches its
// actions and recurses into its `then` chain.
func (e *Engine) evaluate(ctx context.Context, r rule.Rule, ev event.ChangeEvent, fired map[string]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if fired[r.ID] {
		return nil
	}
	if !r.MatchesOperation(ev.Operation) || !matches(r.When, ev) {
		return nil
	}

	fired[r.ID] = true
	e.log.Info("rule fired",
		"rule", r.ID,
		"collection", ev.Collection,
		"operation", ev.Operation,
	)

	if err := e.dispatcher.Dispatch(ctx, r, ev); err != nil {
		// Dispatch already exhausts retries internally; a returned error means
		// the action ultimately failed. Log and continue so the chain and other
		// rules still run.
		e.log.Error("rule dispatch failed", "rule", r.ID, "err", err)
	}

	for _, ref := range r.Then {
		next, ok := e.rules.ByID(ref)
		if !ok {
			// Validation guarantees references resolve; defend anyway.
			continue
		}
		if err := e.evaluate(ctx, next, ev, fired); err != nil {
			return err
		}
	}
	return nil
}
