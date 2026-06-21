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
