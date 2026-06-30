// Package rule defines the declarative trigger schema, its on-disk JSON form,
// loading from a directory, and validation. A rule binds a set of conditions on
// a collection's change events to an ordered list of actions, and may chain to
// further rules to form a directed evaluation graph.
package rule

import (
	"github.com/prabaltripathiofficial/edict/internal/event"
)

// MaxChainedTriggers caps how many rules a single originating event may fire
// through the `Then` chain. This bounds runaway or cyclic rule graphs and is the
// "up to 50 triggers per condition" guarantee from the project goals.
const MaxChainedTriggers = 50

// Operator is a comparison applied by a Condition against an event field.
type Operator string

const (
	OpEq       Operator = "eq"       // field == value
	OpNe       Operator = "ne"       // field != value
	OpGt       Operator = "gt"       // field > value (numeric)
	OpGte      Operator = "gte"      // field >= value (numeric)
	OpLt       Operator = "lt"       // field < value (numeric)
	OpLte      Operator = "lte"      // field <= value (numeric)
	OpIn       Operator = "in"       // field is one of value ([]any)
	OpContains Operator = "contains" // string field contains value substring
	OpExists   Operator = "exists"   // field presence matches value (bool)
)

// Condition is a single field-level predicate. Field is a dot-separated path
// into the event document (see event.ChangeEvent.Lookup).
type Condition struct {
	Field string   `json:"field"`
	Op    Operator `json:"op"`
	Value any      `json:"value"`
}

// Match groups conditions with boolean semantics. All conditions in `All` must
// hold and at least one in `Any` must hold; an empty group is vacuously true.
// A nil Match (no `when` block) matches every event for the collection.
type Match struct {
	All []Condition `json:"all,omitempty"`
	Any []Condition `json:"any,omitempty"`
}

// ActionSpec is the declarative description of one action to run when a rule
// fires. Type selects the handler; the remaining fields are handler-specific and
// validated by the action package at registration/build time.
type ActionSpec struct {
	Type string `json:"type"`

	// Webhook fields.
	URL     string            `json:"url,omitempty"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`

	// Log fields.
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
}

// Rule is a single declarative trigger.
type Rule struct {
	ID          string            `json:"id"`
	Description string            `json:"description,omitempty"`
	Collection  string            `json:"collection"`
	On          []event.Operation `json:"on"`
	When        *Match            `json:"when,omitempty"`
	Actions     []ActionSpec      `json:"actions"`
	// Then names other rule IDs to evaluate after this rule fires, enabling
	// conditional chaining. Chained rules are re-evaluated against the same
	// originating event.
	Then []string `json:"then,omitempty"`
}

// matchesOperation reports whether the rule is interested in op. An empty On
// list means "any operation".
func (r Rule) matchesOperation(op event.Operation) bool {
	if len(r.On) == 0 {
		return true
	}
	for _, o := range r.On {
		if o == op {
			return true
		}
	}
	return false
}

// Set is an indexed collection of rules ready for evaluation.
type Set struct {
	rules  []Rule
	byID   map[string]Rule
	byColl map[string][]Rule
}

// NewSet indexes rules by ID and by collection. It assumes the rules have
// already passed Validate.
func NewSet(rules []Rule) *Set {
	s := &Set{
		rules:  rules,
		byID:   make(map[string]Rule, len(rules)),
		byColl: make(map[string][]Rule),
	}
	for _, r := range rules {
		s.byID[r.ID] = r
		s.byColl[r.Collection] = append(s.byColl[r.Collection], r)
	}
	return s
}

// All returns every rule in the set.
func (s *Set) All() []Rule { return s.rules }

// ByID returns a rule by its ID.
func (s *Set) ByID(id string) (Rule, bool) {
	r, ok := s.byID[id]
	return r, ok
}

// ForCollection returns the rules registered against a collection.
func (s *Set) ForCollection(coll string) []Rule { return s.byColl[coll] }
