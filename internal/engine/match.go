package engine

import (
	"strings"

	"github.com/prabaltripathiofficial/edict/internal/event"
	"github.com/prabaltripathiofficial/edict/internal/rule"
)

// matches reports whether a rule's `when` block holds for an event. A nil match
// (no conditions) is vacuously true. Within a Match, every All condition must
// hold and at least one Any condition must hold; an empty group is skipped.
func matches(m *rule.Match, ev event.ChangeEvent) bool {
	if m == nil {
		return true
	}

	for _, c := range m.All {
		if !evalCondition(c, ev) {
			return false
		}
	}

	if len(m.Any) > 0 {
		anyOK := false
		for _, c := range m.Any {
			if evalCondition(c, ev) {
				anyOK = true
				break
			}
		}
		if !anyOK {
			return false
		}
	}

	return true
}

// evalCondition evaluates a single predicate against the event document.
func evalCondition(c rule.Condition, ev event.ChangeEvent) bool {
	got, present := ev.Lookup(c.Field)

	// Existence is special: it asks about presence, not value, so it must be
	// handled before any value comparison (which would treat a missing field as
	// a non-match for every other operator).
	if c.Op == rule.OpExists {
		want, _ := c.Value.(bool)
		return present == want
	}

	if !present {
		return false
	}

	switch c.Op {
	case rule.OpEq:
		return valuesEqual(got, c.Value)
	case rule.OpNe:
		return !valuesEqual(got, c.Value)
	case rule.OpGt, rule.OpGte, rule.OpLt, rule.OpLte:
		return compareNumeric(c.Op, got, c.Value)
	case rule.OpIn:
		list, ok := c.Value.([]any)
		if !ok {
			return false
		}
		for _, item := range list {
			if valuesEqual(got, item) {
				return true
			}
		}
		return false
	case rule.OpContains:
		gs, ok1 := got.(string)
		vs, ok2 := c.Value.(string)
		return ok1 && ok2 && strings.Contains(gs, vs)
	default:
		return false
	}
}

// valuesEqual compares two values with numeric coercion so that a JSON number
// (always float64) compares equal to an int sourced from a database driver.
func valuesEqual(a, b any) bool {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			return af == bf
		}
	}
	return a == b
}

// compareNumeric applies an ordering operator to two values that must both be
// numeric; non-numeric operands make the condition false rather than panicking.
func compareNumeric(op rule.Operator, got, want any) bool {
	gf, gok := toFloat(got)
	wf, wok := toFloat(want)
	if !gok || !wok {
		return false
	}
	switch op {
	case rule.OpGt:
		return gf > wf
	case rule.OpGte:
		return gf >= wf
	case rule.OpLt:
		return gf < wf
	case rule.OpLte:
		return gf <= wf
	default:
		return false
	}
}

// toFloat coerces the numeric types we expect from JSON rule values and from
// database drivers (which may hand back int32/int64) into a float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
