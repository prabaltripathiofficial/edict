package engine

import (
	"context"
	"testing"

	"github.com/prabaltripathiofficial/edict/internal/event"
	"github.com/prabaltripathiofficial/edict/internal/rule"
)

// recordingDispatcher captures the IDs of rules it was asked to dispatch.
type recordingDispatcher struct {
	fired []string
}

func (d *recordingDispatcher) Dispatch(_ context.Context, r rule.Rule, _ event.ChangeEvent) error {
	d.fired = append(d.fired, r.ID)
	return nil
}

func newEngine(t *testing.T, rules []rule.Rule) (*Engine, *recordingDispatcher) {
	t.Helper()
	if err := rule.Validate(rules); err != nil {
		t.Fatalf("validate rules: %v", err)
	}
	d := &recordingDispatcher{}
	return New(rule.NewSet(rules), d, nil), d
}

func orderEvent(doc map[string]any) event.ChangeEvent {
	return event.ChangeEvent{
		Operation:    event.OpInsert,
		Collection:   "orders",
		FullDocument: doc,
	}
}

func TestHandleEvent_MatchAndChain(t *testing.T) {
	rules := []rule.Rule{
		{
			ID:         "high-value",
			Collection: "orders",
			On:         []event.Operation{event.OpInsert},
			When:       &rule.Match{All: []rule.Condition{{Field: "amount", Op: rule.OpGte, Value: 1000.0}}},
			Actions:    []rule.ActionSpec{{Type: "log"}},
			Then:       []string{"notify"},
		},
		{
			ID:         "notify",
			Collection: "orders",
			Actions:    []rule.ActionSpec{{Type: "log"}},
		},
	}
	e, d := newEngine(t, rules)

	if err := e.HandleEvent(context.Background(), orderEvent(map[string]any{"amount": 1500.0})); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(d.fired) != 2 || d.fired[0] != "high-value" || d.fired[1] != "notify" {
		t.Fatalf("fired = %v, want [high-value notify]", d.fired)
	}
}

func TestHandleEvent_NoMatch(t *testing.T) {
	rules := []rule.Rule{{
		ID:         "high-value",
		Collection: "orders",
		When:       &rule.Match{All: []rule.Condition{{Field: "amount", Op: rule.OpGte, Value: 1000.0}}},
		Actions:    []rule.ActionSpec{{Type: "log"}},
	}}
	e, d := newEngine(t, rules)

	if err := e.HandleEvent(context.Background(), orderEvent(map[string]any{"amount": 50.0})); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(d.fired) != 0 {
		t.Fatalf("fired = %v, want none", d.fired)
	}
}

// A diamond (a→b, a→c, b→d, c→d) must fire d exactly once.
func TestHandleEvent_DiamondFiresOnce(t *testing.T) {
	mk := func(id string, then ...string) rule.Rule {
		return rule.Rule{ID: id, Collection: "orders", Actions: []rule.ActionSpec{{Type: "log"}}, Then: then}
	}
	rules := []rule.Rule{
		mk("a", "b", "c"),
		mk("b", "d"),
		mk("c", "d"),
		mk("d"),
	}
	e, d := newEngine(t, rules)

	if err := e.HandleEvent(context.Background(), orderEvent(nil)); err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	count := 0
	for _, id := range d.fired {
		if id == "d" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("rule d fired %d times, want 1 (fired=%v)", count, d.fired)
	}
}

func TestMatch_Operators(t *testing.T) {
	ev := orderEvent(map[string]any{
		"status": "paid",
		"amount": 1200.0,
		"region": "EU",
		"note":   "priority shipping",
	})
	cases := []struct {
		name string
		c    rule.Condition
		want bool
	}{
		{"eq true", rule.Condition{Field: "status", Op: rule.OpEq, Value: "paid"}, true},
		{"eq false", rule.Condition{Field: "status", Op: rule.OpEq, Value: "pending"}, false},
		{"ne true", rule.Condition{Field: "status", Op: rule.OpNe, Value: "pending"}, true},
		{"gt true", rule.Condition{Field: "amount", Op: rule.OpGt, Value: 1000.0}, true},
		{"lte false", rule.Condition{Field: "amount", Op: rule.OpLte, Value: 1000.0}, false},
		{"in true", rule.Condition{Field: "region", Op: rule.OpIn, Value: []any{"US", "EU"}}, true},
		{"contains true", rule.Condition{Field: "note", Op: rule.OpContains, Value: "priority"}, true},
		{"exists true", rule.Condition{Field: "status", Op: rule.OpExists, Value: true}, true},
		{"exists-false on missing", rule.Condition{Field: "missing", Op: rule.OpExists, Value: false}, true},
		{"missing field non-match", rule.Condition{Field: "missing", Op: rule.OpEq, Value: "x"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := evalCondition(tc.c, ev); got != tc.want {
				t.Errorf("evalCondition(%+v) = %v, want %v", tc.c, got, tc.want)
			}
		})
	}
}
