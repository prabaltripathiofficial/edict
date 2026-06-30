// Package event defines the database-agnostic change event that flows through
// Edict. Source adapters (MongoDB change streams today, others later) normalize
// their native payloads into this shape so the rule engine never has to know
// which database produced an event.
package event

import "time"

// Operation is the kind of change that occurred. The set is intentionally small
// and database-neutral; adapters map their native operation names onto these.
type Operation string

const (
	OpInsert  Operation = "insert"
	OpUpdate  Operation = "update"
	OpReplace Operation = "replace"
	OpDelete  Operation = "delete"
)

// ChangeEvent is the normalized representation of a single data change.
type ChangeEvent struct {
	// Operation is the change kind (insert/update/replace/delete).
	Operation Operation `json:"operation"`
	// Database and Collection identify where the change happened. "Collection"
	// is used generically (table, collection, etc.).
	Database   string `json:"database"`
	Collection string `json:"collection"`
	// DocumentKey is the primary key of the affected document.
	DocumentKey map[string]any `json:"documentKey,omitempty"`
	// FullDocument is the post-change document when available. For deletes it is
	// typically nil; for updates it requires the source to look up the full doc.
	FullDocument map[string]any `json:"fullDocument,omitempty"`
	// UpdatedFields holds only the fields that changed on an update, when the
	// source can provide them. Conditions may match against these directly.
	UpdatedFields map[string]any `json:"updatedFields,omitempty"`
	// Timestamp is when the change was observed.
	Timestamp time.Time `json:"timestamp"`
}

// Lookup resolves a dot-separated field path (e.g. "customer.address.city")
// against the event's document. It checks FullDocument first, then
// UpdatedFields, so rules can be written against the logical document shape
// regardless of whether the source delivered a full or partial payload.
//
// The returned bool reports whether the path existed at all, which lets
// conditions distinguish "field missing" from "field present but nil".
func (e ChangeEvent) Lookup(path string) (any, bool) {
	if v, ok := lookupPath(e.FullDocument, path); ok {
		return v, true
	}
	return lookupPath(e.UpdatedFields, path)
}

// lookupPath walks a nested map using a dot-separated path.
func lookupPath(doc map[string]any, path string) (any, bool) {
	if doc == nil || path == "" {
		return nil, false
	}

	var cur any = doc
	for _, seg := range splitPath(path) {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// splitPath splits "a.b.c" into ["a","b","c"] without pulling in strings.Split
// per-call allocation concerns; paths are short so this stays cheap.
func splitPath(path string) []string {
	segs := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			segs = append(segs, path[start:i])
			start = i + 1
		}
	}
	segs = append(segs, path[start:])
	return segs
}
