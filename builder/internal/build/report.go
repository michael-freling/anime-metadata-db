package build

import (
	"fmt"
	"sort"
	"strings"
)

// Code identifies a kind of finding for machinery that has to act on one —
// the CI gate, chiefly — so that nothing has to match on Message.
//
// Message is prose: it gets reworded, and twice in this feature's history a
// rewording silently disabled the gate that was grepping for it, because a
// pattern that matches nothing looks exactly like a catalogue with nothing
// wrong. A code is an interface, renamed deliberately or not at all.
type Code string

// The finding kinds the anilistId checks produce. Notes that predate this
// carry no code, which is why Code is omitempty rather than required.
const (
	CodeUnlinked       Code = "anilist-unlinked"
	CodeWalkCapped     Code = "anilist-walk-capped"
	CodeOutOfOrder     Code = "anilist-out-of-order"
	CodeDuplicate      Code = "anilist-duplicate"
	CodeTitleDisagrees Code = "anilist-title-disagrees"
)

// gating lists the codes CI fails on. It lives here, beside the checks that
// raise them, rather than in the workflow: which findings are worth stopping a
// merge for is a property of the check, and splitting it across two files is
// how the two drift apart.
//
// CodeWalkCapped is excluded because it says the walk was inconclusive, which
// is not a finding about the id. CodeTitleDisagrees is excluded because it
// depends on how completely upstream lists a series' title among each entry's
// synonyms, which drifts — a gate on that is a gate that fails on someone
// else's edit.
var gating = map[Code]bool{
	CodeUnlinked:   true,
	CodeOutOfOrder: true,
	CodeDuplicate:  true,
}

// Note is one low-confidence decision the builder made (chiefly title-language
// tagging) that a maintainer may want to review and pin with an override.
type Note struct {
	Code    Code   `yaml:"code,omitempty"`
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
// Most ids are resolved from the series' own title rather than authored, and
// the graph check can only corroborate one against its siblings — so a series
// with a single installment has an id nothing verifies. Counting that is the
// difference between "no problems found" and "nothing was looked at", which the
// notes cannot express because the honest output for an unverifiable id is
// silence.
//
// The split between derived and authored is worth reporting on its own. An
// authored id is the residue: the cases upstream does not describe well enough
// to compute, which is where a hand-typed mistake can still enter.
type Coverage struct {
	// Considered is every anilistId the checks saw, derived and authored alike,
	// counted after the resolution has filled what it can. It is the
	// denominator each of the others is a fraction of; without it the counts
	// below divide by whichever check happened to see the id, which is a
	// different number per check.
	Considered int `yaml:"considered"`
	// Corroborated is the number of anilistIds reached from another installment
	// of the same series.
	Corroborated int `yaml:"corroborated"`
	// Alone is the number whose series has no other installment, leaving
	// nothing to check them against.
	Alone int `yaml:"alone"`
	// Derived is the number the build resolved from the series' own title
	// because no override named one. Considered minus Derived is what remains
	// hand-authored.
	Derived int `yaml:"derived"`
	// Agreed is the number an override named and the title resolution
	// independently reproduced, which is the strongest corroboration available:
	// two routes to the same entry. Authoring an id the build could have
	// derived is redundant rather than wrong, and this counts those.
	Agreed int `yaml:"agreed"`
}

// Authored is the number of ids no resolution could supply, so an editor typed
// them. Derived subtracts cleanly because every considered id is one or the
// other: the resolution either filled an empty slot or found one already full.
func (c Coverage) Authored() int { return c.Considered - c.Derived }

// Total is every id the checks considered.
//
// Counted where the ids are collected rather than summed from the outcomes
// below: an id the graph check reports as unlinked is neither corroborated nor
// alone, so adding those two lost exactly the ids most worth counting, and the
// coverage line quietly divided by a denominator that shrank whenever something
// was found.
func (c Coverage) Total() int { return c.Considered }

// Add folds another coverage count into this one.
func (c *Coverage) Add(other Coverage) {
	c.Considered += other.Considered
	c.Corroborated += other.Corroborated
	c.Alone += other.Alone
	c.Derived += other.Derived
	c.Agreed += other.Agreed
}

// add appends a note to the report.
func (r *Report) add(entity, field, message string) {
	r.Notes = append(r.Notes, Note{Entity: entity, Field: field, Message: message})
}

// addCoded appends a note that machinery can recognise without reading it.
func (r *Report) addCoded(code Code, entity, field, message string) {
	r.Notes = append(r.Notes, Note{Code: code, Entity: entity, Field: field, Message: message})
}

// Gating returns the notes CI fails on, in report order.
func (r *Report) Gating() []Note {
	var out []Note
	for _, n := range r.Notes {
		if gating[n.Code] {
			out = append(out, n)
		}
	}
	return out
}

// Codes returns the distinct codes the report carries, in ascending order, so
// a summary line naming them is the same on every run over the same inputs.
func (r *Report) Codes() []Code {
	seen := map[Code]bool{}
	var out []Code
	for _, n := range r.Notes {
		if n.Code != "" && !seen[n.Code] {
			seen[n.Code] = true
			out = append(out, n.Code)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
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
