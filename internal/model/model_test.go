package model

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestReleaseSeasonValid(t *testing.T) {
	for _, s := range []ReleaseSeason{SeasonWinter, SeasonSpring, SeasonSummer, SeasonFall} {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	if ReleaseSeason("AUTUMN").Valid() {
		t.Error("AUTUMN should be invalid")
	}
}

func TestSeasonForMonth(t *testing.T) {
	cases := map[time.Month]ReleaseSeason{
		time.January:   SeasonWinter,
		time.March:     SeasonWinter,
		time.April:     SeasonSpring,
		time.June:      SeasonSpring,
		time.July:      SeasonSummer,
		time.September: SeasonSummer,
		time.October:   SeasonFall,
		time.December:  SeasonFall,
	}
	for m, want := range cases {
		if got := SeasonForMonth(m); got != want {
			t.Errorf("SeasonForMonth(%v) = %v, want %v", m, got, want)
		}
	}
}

func TestSeasonForMonthPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for out-of-range month")
		}
	}()
	SeasonForMonth(time.Month(13))
}

func TestSeasonForDate(t *testing.T) {
	if got := SeasonForDate(time.Date(2019, time.April, 6, 0, 0, 0, 0, time.UTC)); got != SeasonSpring {
		t.Errorf("got %v, want SPRING", got)
	}
}

func TestTitleIsZero(t *testing.T) {
	if !(Title{}).IsZero() {
		t.Error("empty title should be zero")
	}
	if (Title{Original: "x"}).IsZero() {
		t.Error("title with original should not be zero")
	}
	if (Title{Translations: map[string]string{"en": "x"}}).IsZero() {
		t.Error("title with translations should not be zero")
	}
}

func TestExternalIDsIsZero(t *testing.T) {
	if !(ExternalIDs{}).IsZero() {
		t.Error("empty external ids should be zero")
	}
	if (ExternalIDs{AnilistID: 1}).IsZero() {
		t.Error("non-empty external ids should not be zero")
	}
}

func TestSpecialFormatValid(t *testing.T) {
	for _, f := range []SpecialFormat{FormatOVA, FormatONA, FormatSpecial} {
		if !f.Valid() {
			t.Errorf("%q should be valid", f)
		}
	}
	if SpecialFormat("MOVIE").Valid() {
		t.Error("MOVIE should be invalid as a special format")
	}
}

func TestDateRoundTrip(t *testing.T) {
	d := NewDate(2019, time.April, 6)
	out, err := yaml.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "\"2019-04-06\"\n" && string(out) != "2019-04-06\n" {
		t.Fatalf("unexpected marshal output: %q", out)
	}
	var got Date
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Equal(d.Time) {
		t.Errorf("round-trip mismatch: got %v want %v", got.Time, d.Time)
	}
}

func TestDateUnmarshalErrors(t *testing.T) {
	var d Date
	if err := yaml.Unmarshal([]byte("not-a-date"), &d); err == nil {
		t.Error("expected error for malformed date")
	}
	if err := yaml.Unmarshal([]byte("[1, 2]"), &d); err == nil {
		t.Error("expected error for non-string node")
	}
}

func TestRecordEachSeries(t *testing.T) {
	var count int
	count = 0
	(Record{Series: &Series{ID: "a"}}).EachSeries(func(*Series) { count++ })
	if count != 1 {
		t.Errorf("standalone series: visited %d", count)
	}

	count = 0
	(Record{Franchise: &Franchise{Series: []Series{{ID: "a"}, {ID: "b"}}}}).
		EachSeries(func(*Series) { count++ })
	if count != 2 {
		t.Errorf("franchise: visited %d", count)
	}

	count = 0
	(Record{}).EachSeries(func(*Series) { count++ })
	if count != 0 {
		t.Errorf("empty record: visited %d", count)
	}
}

func TestRecordEachSeriesMutates(t *testing.T) {
	rec := Record{Franchise: &Franchise{Series: []Series{{ID: "a"}}}}
	rec.EachSeries(func(s *Series) { s.ID = "changed" })
	if rec.Franchise.Series[0].ID != "changed" {
		t.Error("EachSeries should expose mutable pointers")
	}
}

// A franchise may hold cast at both levels: one character spanning its series,
// another belonging to a single one. Cast has to return both, because
// everything downstream reads the record through it — a character it dropped
// would be validated by nothing, pruned by nothing and served by nothing.
func TestRecordCastSpansBothLevels(t *testing.T) {
	rec := Record{Franchise: &Franchise{
		Characters: []Character{{ID: "spans-the-brand"}},
		Series: []Series{
			{ID: "a", Characters: []Character{{ID: "only-in-a"}}},
			{ID: "b"},
		},
	}}
	got := make([]string, 0, 2)
	for _, c := range rec.Cast() {
		got = append(got, c.ID)
	}
	if len(got) != 2 || got[0] != "spans-the-brand" || got[1] != "only-in-a" {
		t.Errorf("cast = %v, want the franchise character then the series one", got)
	}

	standalone := Record{Series: &Series{Characters: []Character{{ID: "y"}}}}
	if cast := standalone.Cast(); len(cast) != 1 || cast[0].ID != "y" {
		t.Errorf("standalone series cast = %v", cast)
	}
	if cast := (Record{}).Cast(); cast != nil {
		t.Errorf("empty record cast = %v, want nil", cast)
	}
}

// Cast has to copy when a franchise nests cast at both levels, so writing
// through it is not reliable — EachCharacter is what the build uses to fill
// names and default appearances, and it has to reach every character at both
// levels. Getting this wrong loses the fill silently: the record is written
// back to data/ with the copy's changes discarded.
func TestRecordEachCharacterWritesThrough(t *testing.T) {
	rec := Record{Franchise: &Franchise{
		Characters: []Character{{ID: "spans-the-brand"}},
		Series: []Series{
			{ID: "a", Characters: []Character{{ID: "only-in-a"}}},
			{ID: "b", Characters: []Character{{ID: "only-in-b"}}},
		},
	}}
	visited := 0
	rec.EachCharacter(func(c *Character) {
		visited++
		c.Names = Title{Original: "filled " + c.ID}
	})
	if visited != 3 {
		t.Errorf("visited %d characters, want 3", visited)
	}
	if got := rec.Franchise.Characters[0].Names.Original; got != "filled spans-the-brand" {
		t.Errorf("franchise-level character = %q, not written through", got)
	}
	if got := rec.Franchise.Series[0].Characters[0].Names.Original; got != "filled only-in-a" {
		t.Errorf("series-nested character = %q, not written through", got)
	}
	if got := rec.Franchise.Series[1].Characters[0].Names.Original; got != "filled only-in-b" {
		t.Errorf("second series' character = %q, not written through", got)
	}

	standalone := Record{Series: &Series{Characters: []Character{{ID: "y"}}}}
	standalone.EachCharacter(func(c *Character) { c.Names = Title{Original: "filled"} })
	if standalone.Series.Characters[0].Names.Original != "filled" {
		t.Error("standalone series character not written through")
	}

	(Record{}).EachCharacter(func(*Character) { t.Error("empty record visited a character") })
}

// IsRomanization is the shared definition of a convention two modules depend
// on — the builder writes these tags, the API reads them — so it is worth
// pinning directly rather than only through its callers, which all pass the
// same two shapes.
func TestIsRomanization(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		want bool
	}{
		{"ja-Latn", true},
		{"ko-Latn", true},
		{"JA-LATN", true},    // tags are case-insensitive
		{"ja-Latn-JP", true}, // script plus region is a valid tag
		{"ja", false},        // the language itself
		{"en", false},
		{"", false},
		{"latn", false},       // a script with no language names nothing
		{"ja-JP", false},      // a region is not a script
		{"ja-latnish", false}, // the subtag has to be the script, not start with it
	} {
		if got := IsRomanization(tc.tag); got != tc.want {
			t.Errorf("IsRomanization(%q) = %v, want %v", tc.tag, got, tc.want)
		}
	}
}
