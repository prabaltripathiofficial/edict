package rule

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRules drops each named file with the given JSON content into a temp dir
// and returns the dir.
func writeRules(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestLoadDir_Valid(t *testing.T) {
	dir := writeRules(t, map[string]string{
		"a.json": `{
			"id": "high-value-order",
			"collection": "orders",
			"on": ["insert", "update"],
			"when": {"all": [{"field": "amount", "op": "gte", "value": 1000}]},
			"actions": [{"type": "log", "level": "info", "message": "big order"}],
			"then": ["notify-ops"]
		}`,
		"b.json": `{
			"id": "notify-ops",
			"collection": "orders",
			"actions": [{"type": "webhook", "url": "https://example.test/hook"}]
		}`,
	})

	set, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if got := len(set.All()); got != 2 {
		t.Fatalf("loaded %d rules, want 2", got)
	}
	if _, ok := set.ByID("high-value-order"); !ok {
		t.Errorf("expected rule high-value-order to be indexed")
	}
	if got := len(set.ForCollection("orders")); got != 2 {
		t.Errorf("ForCollection(orders) = %d, want 2", got)
	}
}

func TestLoadDir_UnknownField(t *testing.T) {
	dir := writeRules(t, map[string]string{
		"a.json": `{"id": "x", "collection": "c", "actoins": []}`,
	})
	if _, err := LoadDir(dir); err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := []struct {
		name  string
		rules []Rule
	}{
		{
			name:  "empty id",
			rules: []Rule{{Collection: "c", Actions: []ActionSpec{{Type: "log"}}}},
		},
		{
			name:  "missing collection",
			rules: []Rule{{ID: "r", Actions: []ActionSpec{{Type: "log"}}}},
		},
		{
			name:  "no actions",
			rules: []Rule{{ID: "r", Collection: "c"}},
		},
		{
			name: "duplicate id",
			rules: []Rule{
				{ID: "r", Collection: "c", Actions: []ActionSpec{{Type: "log"}}},
				{ID: "r", Collection: "c", Actions: []ActionSpec{{Type: "log"}}},
			},
		},
		{
			name: "dangling then",
			rules: []Rule{
				{ID: "r", Collection: "c", Actions: []ActionSpec{{Type: "log"}}, Then: []string{"ghost"}},
			},
		},
		{
			name: "invalid op",
			rules: []Rule{
				{ID: "r", Collection: "c", Actions: []ActionSpec{{Type: "log"}},
					When: &Match{All: []Condition{{Field: "x", Op: "spaceship", Value: 1}}}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.rules); err == nil {
				t.Fatalf("expected validation error for %q, got nil", tc.name)
			}
		})
	}
}

func TestValidate_CycleRejected(t *testing.T) {
	rules := []Rule{
		{ID: "a", Collection: "c", Actions: []ActionSpec{{Type: "log"}}, Then: []string{"b"}},
		{ID: "b", Collection: "c", Actions: []ActionSpec{{Type: "log"}}, Then: []string{"a"}},
	}
	if err := Validate(rules); err == nil {
		t.Fatal("expected cycle to be rejected, got nil")
	}
}
