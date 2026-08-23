// Package model defines the R1 anime franchise data model:
// Franchise → Series → Season → Episode, plus Movie, Special and WatchOrder.
//
// It mirrors the entities described in the "Anime Series Data Model" design
// note. The types are plain data with a handful of pure helpers; all building
// and validation logic lives in the build package.
package model

import (
	"strings"
	"time"
)

// IsRomanization reports whether a BCP-47 tag names a Latin-script rendering of
// a language written in another script — "ja-Latn", "ko-Latn". It is the one
// definition of that convention: the builder writes such tags and the API reads
// them, from separate modules, and two copies of the rule would drift.
func IsRomanization(tag string) bool {
	return strings.HasSuffix(strings.ToLower(tag), "-latn")
}

// ReleaseSeason is the airing quarter an installment premiered in. It is a
// calendar quarter, distinct from the Season entity (a TV installment).
type ReleaseSeason string

// The four airing quarters.
const (
	SeasonWinter ReleaseSeason = "WINTER"
	SeasonSpring ReleaseSeason = "SPRING"
	SeasonSummer ReleaseSeason = "SUMMER"
	SeasonFall   ReleaseSeason = "FALL"
)

// Valid reports whether s is one of the four recognised quarters.
func (s ReleaseSeason) Valid() bool {
	switch s {
	case SeasonWinter, SeasonSpring, SeasonSummer, SeasonFall:
		return true
	default:
		return false
	}
}

// SeasonForMonth maps a calendar month (1-12) to its airing quarter using the
// naive month map: Jan–Mar = Winter, Apr–Jun = Spring, Jul–Sep = Summer,
// Oct–Dec = Fall. It panics for an out-of-range month.
func SeasonForMonth(month time.Month) ReleaseSeason {
	switch {
	case month >= time.January && month <= time.March:
		return SeasonWinter
	case month >= time.April && month <= time.June:
		return SeasonSpring
	case month >= time.July && month <= time.September:
		return SeasonSummer
	case month >= time.October && month <= time.December:
		return SeasonFall
	default:
		panic("model: month out of range")
	}
}

// SeasonForDate returns the airing quarter for a release date.
func SeasonForDate(t time.Time) ReleaseSeason {
	return SeasonForMonth(t.Month())
}

// Title holds a localized title or name: the original native-script form plus
// a map of translations keyed by BCP-47 code (en, ja-Latn, ko, …).
type Title struct {
	Original     string            `yaml:"original,omitempty"`
	Translations map[string]string `yaml:"translations,omitempty"`
}

// IsZero reports whether the title carries no information.
func (t Title) IsZero() bool {
	return t.Original == "" && len(t.Translations) == 0
}

// ExternalIDs cross-maps a node to external databases. AnilistID is the primary
// join key for media and R2 nodes; WikidataID (a QID) is the build-time key for
// characters and staff. All are optional.
type ExternalIDs struct {
	AnilistID  int    `yaml:"anilistId,omitempty"`
	AnidbID    int    `yaml:"anidbId,omitempty"`
	TmdbID     int    `yaml:"tmdbId,omitempty"`
	TvdbID     int    `yaml:"tvdbId,omitempty"`
	WikidataID string `yaml:"wikidataId,omitempty"`
}

// IsZero reports whether no external id is set, so the field is omitted from
// output when empty.
func (e ExternalIDs) IsZero() bool {
	return e == ExternalIDs{}
}

// Date is a calendar date serialised as YYYY-MM-DD in YAML.
type Date struct {
	time.Time
}

// dateLayout is the canonical wire format for Date.
const dateLayout = "2006-01-02"

// NewDate constructs a Date from year, month and day in UTC.
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate reads the canonical YYYY-MM-DD form that MarshalYAML writes.
func ParseDate(s string) (*Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil, err
	}
	return &Date{t}, nil
}

// MarshalYAML renders the date as YYYY-MM-DD.
func (d Date) MarshalYAML() (any, error) {
	return d.Format(dateLayout), nil
}

// UnmarshalYAML parses a YYYY-MM-DD scalar.
func (d *Date) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = *parsed
	return nil
}

// Episode is one TV episode.
type Episode struct {
	// AbsoluteNumber is the continuous count across a numbered Series. It is
	// nil for non-numbered series.
	AbsoluteNumber *int `yaml:"absoluteNumber,omitempty"`
	// AiredNumber is the local number within this season/part.
	AiredNumber int    `yaml:"airedNumber"`
	ReleaseDate *Date  `yaml:"releaseDate,omitempty"`
	Title       string `yaml:"title,omitempty"`
}

// Season is one numbered TV installment (one media node / cour).
type Season struct {
	ID            string        `yaml:"id"`
	Titles        Title         `yaml:"titles,omitempty"`
	Number        int           `yaml:"number"`
	Part          *int          `yaml:"part,omitempty"`
	ReleaseDate   *Date         `yaml:"releaseDate,omitempty"`
	ReleaseYear   int           `yaml:"releaseYear,omitempty"`
	ReleaseSeason ReleaseSeason `yaml:"releaseSeason,omitempty"`
	ExternalIDs   ExternalIDs   `yaml:"externalIds,omitempty"`
	Episodes      []Episode     `yaml:"episodes,omitempty"`
}

// AlternateCutOf links an alternate-cut film to the Season that carries its
// numbering.
type AlternateCutOf struct {
	SeasonID string `yaml:"seasonId"`
	Episodes string `yaml:"episodes,omitempty"`
}

// Movie is one film (one media node).
type Movie struct {
	ID             string          `yaml:"id"`
	Titles         Title           `yaml:"titles,omitempty"`
	ReleaseDate    *Date           `yaml:"releaseDate,omitempty"`
	ReleaseYear    int             `yaml:"releaseYear,omitempty"`
	ExternalIDs    ExternalIDs     `yaml:"externalIds,omitempty"`
	AbsoluteNumber *int            `yaml:"absoluteNumber,omitempty"`
	AlternateCutOf *AlternateCutOf `yaml:"alternateCutOf,omitempty"`
}

// SpecialFormat is the kind of side content a Special represents.
type SpecialFormat string

// The recognised special formats.
const (
	FormatOVA     SpecialFormat = "OVA"
	FormatONA     SpecialFormat = "ONA"
	FormatSpecial SpecialFormat = "SPECIAL"
)

// Valid reports whether f is a recognised special format.
func (f SpecialFormat) Valid() bool {
	switch f {
	case FormatOVA, FormatONA, FormatSpecial:
		return true
	default:
		return false
	}
}

// Special is one OVA / ONA / special (side content, no season number).
type Special struct {
	ID             string        `yaml:"id"`
	Titles         Title         `yaml:"titles,omitempty"`
	Format         SpecialFormat `yaml:"format"`
	ReleaseDate    *Date         `yaml:"releaseDate,omitempty"`
	ReleaseYear    int           `yaml:"releaseYear,omitempty"`
	ExternalIDs    ExternalIDs   `yaml:"externalIds,omitempty"`
	Episodes       []Episode     `yaml:"episodes,omitempty"`
	AbsoluteNumber *int          `yaml:"absoluteNumber,omitempty"`
}

// Series is the base unit: one storyline / continuity.
type Series struct {
	ID       string    `yaml:"id"`
	Titles   Title     `yaml:"titles,omitempty"`
	Seasons  []Season  `yaml:"seasons,omitempty"`
	Movies   []Movie   `yaml:"movies,omitempty"`
	Specials []Special `yaml:"specials,omitempty"`
	// Characters is the cast nested under this standalone series (R2).
	Characters []Character `yaml:"characters,omitempty"`
}

// WatchOrderEntry is one ordered reference within a WatchOrder.
type WatchOrderEntry struct {
	Ref  string `yaml:"ref"`
	Note string `yaml:"note,omitempty"`
}

// WatchOrder is a named curated alternate order across a Franchise's Series.
type WatchOrder struct {
	Name    string            `yaml:"name"`
	Entries []WatchOrderEntry `yaml:"entries"`
}

// Franchise groups related Series under one brand. It is present only when a
// brand has several independent storylines.
type Franchise struct {
	ID          string       `yaml:"id"`
	Titles      Title        `yaml:"titles,omitempty"`
	Series      []Series     `yaml:"series"`
	WatchOrders []WatchOrder `yaml:"watchOrders,omitempty"`
	// Characters is the cast nested under this franchise (R2). Franchise-level
	// because a character spans the franchise's series.
	Characters []Character `yaml:"characters,omitempty"`
}

// Record is one generated dataset file: a Franchise or Series (R1 structure)
// with its cast (R2) nested inside it. It is the canonical output shape the
// writer emits into data/series/.
type Record struct {
	Franchise *Franchise `yaml:"franchise,omitempty"`
	Series    *Series    `yaml:"series,omitempty"`
}

// EachSeries calls fn for every Series in the record (the single standalone
// series, or each series of the franchise).
func (r Record) EachSeries(fn func(*Series)) {
	switch {
	case r.Series != nil:
		fn(r.Series)
	case r.Franchise != nil:
		for i := range r.Franchise.Series {
			fn(&r.Franchise.Series[i])
		}
	}
}

// Cast returns every character in the record: nested under the franchise (for
// one spanning its series) and under each series it holds, or under the
// standalone series. Everything downstream — the index, the store, the
// builder's validation and prune accounting — reads the cast through here, so
// a character this missed would be silently unservable rather than rejected.
//
// It is for READING. A franchise nesting cast at both levels has no single
// slice to hand back, so this one allocates and copies, and writing to an
// element of the result would be lost. Use EachCharacter to modify.
func (r Record) Cast() []Character {
	switch {
	case r.Franchise != nil:
		nested := 0
		for _, s := range r.Franchise.Series {
			nested += len(s.Characters)
		}
		if nested == 0 {
			return r.Franchise.Characters
		}
		out := make([]Character, 0, len(r.Franchise.Characters)+nested)
		out = append(out, r.Franchise.Characters...)
		for _, s := range r.Franchise.Series {
			out = append(out, s.Characters...)
		}
		return out
	case r.Series != nil:
		return r.Series.Characters
	}
	return nil
}

// EachCharacter calls fn for every character in the record, by pointer, so the
// record itself is what changes. It is the writing counterpart to Cast: the
// build fills names and default appearances through here, and doing that
// through Cast would silently discard the work for a franchise that nests cast
// at both levels, since Cast has to copy to return one slice.
func (r Record) EachCharacter(fn func(*Character)) {
	switch {
	case r.Franchise != nil:
		for i := range r.Franchise.Characters {
			fn(&r.Franchise.Characters[i])
		}
		for i := range r.Franchise.Series {
			for j := range r.Franchise.Series[i].Characters {
				fn(&r.Franchise.Series[i].Characters[j])
			}
		}
	case r.Series != nil:
		for i := range r.Series.Characters {
			fn(&r.Series.Characters[i])
		}
	}
}
