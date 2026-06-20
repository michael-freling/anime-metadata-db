package build

import (
	"fmt"
	"sort"
	"strings"
)

// Note is one low-confidence decision the builder made (chiefly title-language
// tagging) that a maintainer may want to review and pin with an override.
type Note struct {
	Entity  string `yaml:"entity"`
	Field   string `yaml:"field,omitempty"`
	Message string `yaml:"message"`
}

// Report collects the build's low-confidence decisions (design Part 4).
type Report struct {
	Notes []Note `yaml:"notes"`
}

// add appends a note to the report.
func (r *Report) add(entity, field, message string) {
	r.Notes = append(r.Notes, Note{Entity: entity, Field: field, Message: message})
}

// Empty reports whether the report has no notes.
func (r *Report) Empty() bool { return len(r.Notes) == 0 }

// Merge folds another report's notes into this one.
func (r *Report) Merge(other *Report) {
	if other == nil {
		return
	}
	r.Notes = append(r.Notes, other.Notes...)
}

// Sort orders notes by entity then field for deterministic output.
func (r *Report) Sort() {
	sort.SliceStable(r.Notes, func(i, j int) bool {
		if r.Notes[i].Entity != r.Notes[j].Entity {
			return r.Notes[i].Entity < r.Notes[j].Entity
		}
		return r.Notes[i].Field < r.Notes[j].Field
	})
}

// String renders the report as human-readable warning lines for stdout.
func (r *Report) String() string {
	if r.Empty() {
		return ""
	}
	var b strings.Builder
	for _, n := range r.Notes {
		if n.Field != "" {
			fmt.Fprintf(&b, "  %s [%s]: %s\n", n.Entity, n.Field, n.Message)
		} else {
			fmt.Fprintf(&b, "  %s: %s\n", n.Entity, n.Message)
		}
	}
	return b.String()
}
