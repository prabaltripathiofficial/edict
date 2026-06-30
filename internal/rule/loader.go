package rule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadDir reads every *.json file in dir, decodes each into a Rule, validates
// the resulting set, and returns an indexed Set. Files are processed in sorted
// order so behavior is deterministic across runs and platforms.
//
// A single malformed or invalid file fails the whole load: partially-applied
// rule sets are worse than none, because a missing rule changes which side
// effects fire.
func LoadDir(dir string) (*Set, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read rules dir %q: %w", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	rules := make([]Rule, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		r, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}

	if err := Validate(rules); err != nil {
		return nil, err
	}
	return NewSet(rules), nil
}

// loadFile decodes a single rule file. DisallowUnknownFields catches typos in
// rule authoring (e.g. "actoins") that would otherwise be silently ignored.
func loadFile(path string) (Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Rule{}, fmt.Errorf("read rule file %q: %w", path, err)
	}

	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()

	var r Rule
	if err := dec.Decode(&r); err != nil {
		return Rule{}, fmt.Errorf("parse rule file %q: %w", path, err)
	}
	return r, nil
}
