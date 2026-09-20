package build

import (
	"strings"
	"testing"
)

func TestReportBasics(t *testing.T) {
	r := &Report{}
	if !r.Empty() {
		t.Error("new report should be empty")
	}
	r.add("season x", "titles", "guessed original")
	r.add("movie y", "", "in a movie set")
	if r.Empty() {
		t.Error("report should not be empty after add")
	}
}

func TestReportMerge(t *testing.T) {
	r := &Report{}
	r.Merge(nil) // no-op
	other := &Report{}
	other.add("a", "f", "m")
	r.Merge(other)
	if len(r.Notes) != 1 {
		t.Errorf("expected 1 note after merge, got %d", len(r.Notes))
	}
}

func TestReportSortAndString(t *testing.T) {
	r := &Report{}
	r.add("zebra", "titles", "z")
	r.add("apple", "format", "a")
	r.add("apple", "aaa", "a2")
	r.Sort()
	if r.Notes[0].Entity != "apple" || r.Notes[0].Field != "aaa" {
		t.Errorf("sort order wrong: %+v", r.Notes)
	}

	s := r.String()
	if !strings.Contains(s, "apple [aaa]: a2") {
		t.Errorf("string missing field form: %q", s)
	}

	// Note without a field renders without brackets.
	noField := &Report{}
	noField.add("movie y", "", "in a set")
	if got := noField.String(); !strings.Contains(got, "movie y: in a set") {
		t.Errorf("string missing no-field form: %q", got)
	}

	// Empty report renders to empty string.
	if (&Report{}).String() != "" {
		t.Error("empty report should render empty string")
	}
}

// The CI gate reads these two, so what they return is an interface rather than
// an implementation detail: a change here retires or fires a gate.
func TestGatingAndCodes(t *testing.T) {
	r := &Report{}
	r.add("series x", "titles", "a note with no code at all")
	r.addCoded(CodeWalkCapped, "season x-s1", "externalIds", "inconclusive")
	r.addCoded(CodeUnlinked, "season x-s2", "externalIds", "not linked")
	r.addCoded(CodeTitleDisagrees, "season x-s2", "externalIds", "disagrees")
	r.addCoded(CodeDuplicate, "series x", "externalIds", "twice")

	gating := r.Gating()
	if len(gating) != 2 {
		t.Fatalf("two of the five gate, got %d: %+v", len(gating), gating)
	}
	for _, n := range gating {
		if n.Code != CodeUnlinked && n.Code != CodeDuplicate {
			t.Errorf("%s must not gate", n.Code)
		}
	}
	// Named individually rather than counted, because excluding these two is a
	// deliberate decision with a reason recorded beside `gating`, and a count
	// alone would still pass if the wrong two had been excluded.
	for _, c := range []Code{CodeWalkCapped, CodeTitleDisagrees} {
		for _, n := range gating {
			if n.Code == c {
				t.Errorf("%s gates on upstream's own drift, so it must not fail CI", c)
			}
		}
	}

	// Codes reports every kind raised, gating or not, so the planted-error test
	// can name which check fired — sorted, and each one once.
	want := []Code{CodeDuplicate, CodeTitleDisagrees, CodeUnlinked, CodeWalkCapped}
	got := r.Codes()
	if len(got) != len(want) {
		t.Fatalf("codes: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("codes must be sorted and distinct: got %v, want %v", got, want)
		}
	}

	// An uncoded note is not a finding, so a report of nothing but those gates
	// on nothing and names nothing.
	plain := &Report{}
	plain.add("series y", "titles", "guessed a language")
	if g := plain.Gating(); len(g) != 0 {
		t.Errorf("uncoded notes do not gate, got %+v", g)
	}
	if c := plain.Codes(); len(c) != 0 {
		t.Errorf("uncoded notes name no codes, got %v", c)
	}
}
