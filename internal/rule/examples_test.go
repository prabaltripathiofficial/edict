package rule_test

import (
	"testing"

	"github.com/prabaltripathiofficial/edict/internal/rule"
)

// TestExampleRulesLoad guards the rules shipped in the repo's rules/ directory:
// they must always parse and pass validation so the project's own examples
// never rot.
func TestExampleRulesLoad(t *testing.T) {
	set, err := rule.LoadDir("../../rules")
	if err != nil {
		t.Fatalf("example rules failed to load: %v", err)
	}
	if len(set.All()) == 0 {
		t.Fatal("expected at least one example rule")
	}
}
