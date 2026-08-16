package api

import (
	"testing"
	"testing/fstest"
)

// franchiseYAML exercises every R1 shape: original+translated titles, seasons
// with part/releaseDate/all release-season enums (plus an unknown), episodes
// with and without absoluteNumber, movies with alternateCutOf, specials of
// every format (plus an unknown), and watch orders.
const franchiseYAML = `franchise:
  id: aaa
  titles:
    original: エー
    translations:
      en: Alpha Franchise
  series:
    - id: aaa-main
      titles:
        translations:
          en: Alpha Main
      seasons:
        - id: aaa-s1
          number: 1
          part: 1
          releaseYear: 2006
          releaseSeason: WINTER
          releaseDate: 2006-01-06
          externalIds:
            anilistId: 1
            anidbId: 2
            tmdbId: 3
            tvdbId: 4
            wikidataId: Q1
          episodes:
            - absoluteNumber: 1
              airedNumber: 1
              releaseDate: 2006-01-06
              title: Pilot
            - airedNumber: 2
        - id: aaa-s2
          number: 2
          releaseSeason: SPRING
        - id: aaa-s3
          number: 3
          releaseSeason: SUMMER
        - id: aaa-s4
          number: 4
          releaseSeason: BOGUS
      movies:
        - id: aaa-movie
          titles:
            translations:
              en: Alpha Movie
          releaseYear: 2010
          absoluteNumber: 5
          externalIds:
            anilistId: 10
          alternateCutOf:
            seasonId: aaa-s1
            episodes: 1-13
      specials:
        - id: aaa-ova
          format: OVA
          absoluteNumber: 6
          episodes:
            - airedNumber: 1
        - id: aaa-ona
          format: ONA
        - id: aaa-sp
          format: SPECIAL
        - id: aaa-unknown
          format: WAT
  watchOrders:
    - name: Release
      entries:
        - ref: aaa-s1
          note: start here
        - ref: aaa-s2
`

// standaloneYAML is a top-level series (no franchise) using the FALL enum.
const standaloneYAML = `series:
  id: zzz
  titles:
    original: ゼッド
    translations:
      en: Zed Standalone
  seasons:
    - id: zzz-s1
      number: 1
      releaseSeason: FALL
`

// minimalYAML is a series with no titles or installments, covering the empty
// branches of the converters and store.
const minimalYAML = `series:
  id: minimal
`

// newTestFS returns an fs.FS with the three fixtures plus a non-YAML file that
// must be ignored.
func newTestFS() fstest.MapFS {
	return fstest.MapFS{
		"data/series/aaa.yaml":  {Data: []byte(franchiseYAML)},
		"data/series/zzz.yaml":  {Data: []byte(standaloneYAML)},
		"data/series/min.yaml":  {Data: []byte(minimalYAML)},
		"data/series/README.md": {Data: []byte("not yaml")},
	}
}

// mustStore builds a Store from the standard test fixtures.
func mustStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(newTestFS())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func TestNewStoreStats(t *testing.T) {
	s := mustStore(t)
	got := s.Stats()
	want := Stats{Franchises: 1, Series: 3, Seasons: 5, Episodes: 3}
	if got != want {
		t.Errorf("Stats() = %+v, want %+v", got, want)
	}
}

func TestStoreFranchises(t *testing.T) {
	s := mustStore(t)
	fs := s.Franchises()
	if len(fs) != 1 || fs[0].ID != "aaa" {
		t.Fatalf("Franchises() = %v, want one franchise aaa", fs)
	}
	f, ok := s.Franchise("aaa")
	if !ok || f.ID != "aaa" {
		t.Fatalf("Franchise(aaa) = %v, %v", f, ok)
	}
	if _, ok := s.Franchise("nope"); ok {
		t.Error("Franchise(nope) should not be found")
	}
	// A series id is not a franchise id.
	if _, ok := s.Franchise("aaa-main"); ok {
		t.Error("Franchise(aaa-main) should not be found")
	}
}

func TestStoreSeries(t *testing.T) {
	s := mustStore(t)
	// Series under a franchise.
	series, fid, ok := s.Series("aaa-main")
	if !ok || series.ID != "aaa-main" || fid != "aaa" {
		t.Fatalf("Series(aaa-main) = %v, %q, %v", series, fid, ok)
	}
	// Standalone series: empty franchise id.
	series, fid, ok = s.Series("zzz")
	if !ok || series.ID != "zzz" || fid != "" {
		t.Fatalf("Series(zzz) = %v, %q, %v", series, fid, ok)
	}
	if _, _, ok := s.Series("missing"); ok {
		t.Error("Series(missing) should not be found")
	}
}

func TestStoreSearch(t *testing.T) {
	s := mustStore(t)
	tests := []struct {
		name    string
		query   string
		limit   int
		wantIDs []string
	}{
		{"translation match", "alpha", 0, []string{"aaa", "aaa-main"}},
		{"original-script match", "ゼッド", 0, []string{"zzz"}},
		{"case-insensitive", "ZED", 0, []string{"zzz"}},
		{"limit caps results", "a", 1, []string{"aaa"}},
		{"blank query", "  ", 0, nil},
		{"no match", "nothinghere", 0, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := s.Search(tc.query, tc.limit)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("Search(%q, %d) = %d results, want %d", tc.query, tc.limit, len(got), len(tc.wantIDs))
			}
			for i, e := range got {
				if e.ID != tc.wantIDs[i] {
					t.Errorf("result %d id = %q, want %q", i, e.ID, tc.wantIDs[i])
				}
			}
		})
	}
}

func TestNewStoreErrors(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
	}{
		{"missing data dir", fstest.MapFS{"other.txt": {Data: []byte("x")}}},
		{"malformed yaml", fstest.MapFS{"data/series/bad.yaml": {Data: []byte("franchise: [unterminated")}}},
		{"empty record", fstest.MapFS{"data/series/empty.yaml": {Data: []byte("{}")}}},
		{"duplicate franchise", fstest.MapFS{
			"data/series/a.yaml": {Data: []byte("franchise:\n  id: dup\n")},
			"data/series/b.yaml": {Data: []byte("franchise:\n  id: dup\n")},
		}},
		{"duplicate series", fstest.MapFS{
			"data/series/a.yaml": {Data: []byte("series:\n  id: dup\n")},
			"data/series/b.yaml": {Data: []byte("series:\n  id: dup\n")},
		}},
		{"duplicate nested series", fstest.MapFS{
			"data/series/a.yaml": {Data: []byte("franchise:\n  id: f\n  series:\n    - id: s\n    - id: s\n")},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewStore(tc.fsys); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
