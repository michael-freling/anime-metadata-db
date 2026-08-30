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
	// Coverage is what the checks could see, as opposed to what they found.
	// A build that reports nothing is either clean or blind, and the two look
	// identical from the notes alone.
	Coverage Coverage `yaml:"coverage"`
}

// Coverage counts how far the anilistId checks reached.
//
// Every installment's id is authored by hand, and the graph check can only
// corroborate one against its siblings — so a series with a single installment
// has an id nothing verifies. Counting that is the difference between "no
// problems found" and "nothing was looked at", which the notes cannot express
// because the honest output for an unverifiable id is silence.
type Coverage struct {
	// Authored is every anilistId the checks considered, whether or not any of
	// them could say anything about it. It is the denominator each of the
	// others is a fraction of; without it the counts below divide by whichever
	// check happened to see the id, which is a different number per check.
	Authored int `yaml:"authored"`
	// Corroborated is the number of authored anilistIds reached from another
	// installment of the same series.
	Corroborated int `yaml:"corroborated"`
	// Alone is the number whose series has no other installment, leaving
	// nothing to check them against.
	Alone int `yaml:"alone"`
	// Derived is the number the build resolved from the series' own title
	// because no override named one.
	Derived int `yaml:"derived"`
	// Agreed is the number an authored id and the title resolution both name,
	// which is the strongest corroboration available: two independent routes to
	// the same entry.
	Agreed int `yaml:"agreed"`
}

// Total is every id the checks considered.
//
// Counted where the ids are collected rather than summed from the outcomes
// below: an id the graph check reports as unlinked is neither corroborated nor
// alone, so adding those two lost exactly the ids most worth counting, and the
// coverage line quietly divided by a denominator that shrank whenever something
// was found.
func (c Coverage) Total() int { return c.Authored }

// Add folds another coverage count into this one.
func (c *Coverage) Add(other Coverage) {
	c.Authored += other.Authored
	c.Corroborated += other.Corroborated
	c.Alone += other.Alone
	c.Derived += other.Derived
	c.Agreed += other.Agreed
}

// add appends a note to the report.
func (r *Report) add(entity, field, message string) {
	r.Notes = append(r.Notes, Note{Entity: entity, Field: field, Message: message})
}

// Empty reports whether the report has no notes. Coverage is deliberately not
// consulted: a build that checked a great deal and found nothing has an empty
// report, which is the point.
func (r *Report) Empty() bool { return len(r.Notes) == 0 }

// Merge folds another report's notes into this one.
func (r *Report) Merge(other *Report) {
	if other == nil {
		return
	}
	r.Notes = append(r.Notes, other.Notes...)
	r.Coverage.Add(other.Coverage)
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
