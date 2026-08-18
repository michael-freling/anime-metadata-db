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
