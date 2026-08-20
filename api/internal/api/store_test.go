package api

import (
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/michael-freling/anime-metadata-db/api/internal/index"
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
  characters:
    - id: alpha-hero
      names:
        original: アルファ
        translations:
          en: Alpha Hero
      externalIds:
        wikidataId: Q100
      voiceActors:
        - staffId: va-one
          language: ja
      appearances:
        # Scoped to one season of the owning series.
        - seriesId: aaa-main
          scope:
            - seasonId: aaa-s1
            - movieId: aaa-movie
            - specialId: aaa-ova
        # Cross-record appearance that overrides the default cast.
        - seriesId: zzz
          externalIds:
            anilistId: 99
          voiceActors:
            - staffId: va-two
              language: en
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
  characters:
    # No appearances: the default cast still yields a credit, with no series.
    - id: zed-friend
      names:
        translations:
          en: Zed Friend
      voiceActors:
        - staffId: va-one
          language: en
`

// staffYAML is the global staff file: two voice actors, one of them unnamed.
const staffYAML = `staff:
  - id: va-one
    names:
      original: 声優一
      translations:
        en: Voice One
    externalIds:
      wikidataId: Q200
  - id: va-two
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
		"data/staff/va.yaml":    {Data: []byte(staffYAML)},
		"data/staff/notes.txt":  {Data: []byte("not yaml")},
	}
}

// mustStore builds a Store from the standard test fixtures.
func mustStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStoreFromDataset(newTestFS())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func TestNewStoreStats(t *testing.T) {
	s := mustStore(t)
	got := s.Stats()
	want := Stats{
		Franchises: 1, Series: 3, Seasons: 5, Episodes: 3, Characters: 2, Staff: 2,
		EarliestReleaseYear: 2006, LatestReleaseYear: 2010,
	}
	if got != want {
		t.Errorf("Stats() = %+v, want %+v", got, want)
	}
}

func TestStoreFranchises(t *testing.T) {
	s := mustStore(t)
	page, err := s.FranchisesPage("", 0)
	if err != nil {
		t.Fatalf("FranchisesPage: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "aaa" || page.Total != 1 {
		t.Fatalf("FranchisesPage() = %v (total %d), want one franchise aaa", page.Items, page.Total)
	}
	f, ok, err := s.Franchise("aaa")
	if err != nil || !ok || f.ID != "aaa" {
		t.Fatalf("Franchise(aaa) = %v, %v, %v", f, ok, err)
	}
	if _, ok, _ := s.Franchise("nope"); ok {
		t.Error("Franchise(nope) should not be found")
	}
	// A series id is not a franchise id.
	if _, ok, _ := s.Franchise("aaa-main"); ok {
		t.Error("Franchise(aaa-main) should not be found")
	}
}

func TestStoreSeries(t *testing.T) {
	s := mustStore(t)
	// Series under a franchise.
	series, fid, ok, err := s.Series("aaa-main")
	if err != nil || !ok || series.ID != "aaa-main" || fid != "aaa" {
		t.Fatalf("Series(aaa-main) = %v, %q, %v, %v", series, fid, ok, err)
	}
	// Standalone series: empty franchise id.
	series, fid, ok, err = s.Series("zzz")
	if err != nil || !ok || series.ID != "zzz" || fid != "" {
		t.Fatalf("Series(zzz) = %v, %q, %v, %v", series, fid, ok, err)
	}
	if _, _, ok, _ := s.Series("missing"); ok {
		t.Error("Series(missing) should not be found")
	}
	if !s.SeriesExists("zzz") || s.SeriesExists("missing") {
		t.Error("SeriesExists disagrees with Series")
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
			page, err := s.SearchPage(tc.query, "", tc.limit)
			if err != nil {
				t.Fatalf("SearchPage(%q, %d): %v", tc.query, tc.limit, err)
			}
			got := page.Items
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("SearchPage(%q, %d) = %d results, want %d", tc.query, tc.limit, len(got), len(tc.wantIDs))
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
		{"duplicate character across files", fstest.MapFS{
			"data/series/a.yaml": {Data: []byte("series:\n  id: a\n  characters:\n    - id: dup\n")},
			"data/series/b.yaml": {Data: []byte("series:\n  id: b\n  characters:\n    - id: dup\n")},
		}},
		{"malformed staff yaml", fstest.MapFS{
			"data/series/a.yaml": {Data: []byte("series:\n  id: a\n")},
			"data/staff/s.yaml":  {Data: []byte("staff: [unterminated")},
		}},
		{"duplicate staff id", fstest.MapFS{
			"data/series/a.yaml": {Data: []byte("series:\n  id: a\n")},
			"data/staff/x.yaml":  {Data: []byte("staff:\n  - id: dup\n")},
			"data/staff/y.yaml":  {Data: []byte("staff:\n  - id: dup\n")},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewStoreFromDataset(tc.fsys); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// A dataset with no data/staff subtree loads fine — staff are optional.
func TestNewStoreWithoutStaffDir(t *testing.T) {
	s, err := NewStoreFromDataset(fstest.MapFS{"data/series/a.yaml": {Data: []byte("series:\n  id: a\n")}})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if got := s.Stats().Staff; got != 0 {
		t.Errorf("Staff = %d, want 0", got)
	}
	page, err := s.StaffPage("", "", "", 0)
	if err != nil {
		t.Fatalf("StaffPage: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("StaffPage() = %v, want empty", page.Items)
	}
}

func TestStoreCharacters(t *testing.T) {
	s := mustStore(t)

	c, ok, err := s.Character("alpha-hero")
	if err != nil || !ok || c.Names.Translations["en"] != "Alpha Hero" {
		t.Fatalf("Character(alpha-hero) = %+v, %v, %v", c, ok, err)
	}
	if _, ok, _ := s.Character("nobody"); ok {
		t.Error("Character(nobody) should not be found")
	}

	tests := []struct {
		name     string
		seriesID string
		limit    int
		wantIDs  []string
	}{
		{"whole cast", "", 0, []string{"alpha-hero", "zed-friend"}},
		{"owning series", "aaa-main", 0, []string{"alpha-hero"}},
		// The cross-record appearance files the character under zzz too.
		{"appearance in another record", "zzz", 0, []string{"alpha-hero"}},
		{"series with no cast", "minimal", 0, nil},
		{"unknown series", "nope", 0, nil},
		{"limit caps results", "", 1, []string{"alpha-hero"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.Characters(tc.seriesID, tc.limit)
			if err != nil {
				t.Fatalf("Characters(%q, %d): %v", tc.seriesID, tc.limit, err)
			}
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("Characters(%q, %d) = %d, want %d", tc.seriesID, tc.limit, len(got), len(tc.wantIDs))
			}
			for i, c := range got {
				if c.ID != tc.wantIDs[i] {
					t.Errorf("character %d = %q, want %q", i, c.ID, tc.wantIDs[i])
				}
			}
		})
	}
}

func TestStoreStaff(t *testing.T) {
	s := mustStore(t)

	st, ok := s.Staff("va-one")
	if !ok || st.ExternalIDs.WikidataID != "Q200" {
		t.Fatalf("Staff(va-one) = %+v, %v", st, ok)
	}
	if _, ok := s.Staff("va-none"); ok {
		t.Error("Staff(va-none) should not be found")
	}

	tests := []struct {
		name     string
		language string
		limit    int
		wantIDs  []string
	}{
		{"everyone", "", 0, []string{"va-one", "va-two"}},
		// va-one is cast in ja (alpha-hero) and en (zed-friend).
		{"japanese credits", "ja", 0, []string{"va-one"}},
		{"english credits", "en", 0, []string{"va-one", "va-two"}},
		{"language with no credits", "fr", 0, nil},
		{"limit caps results", "", 1, []string{"va-one"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.StaffPage(tc.language, "", "", tc.limit)
			if err != nil {
				t.Fatalf("StaffPage(%q, %d): %v", tc.language, tc.limit, err)
			}
			got := page.Items
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("StaffPage(%q, %d) = %d, want %d", tc.language, tc.limit, len(got), len(tc.wantIDs))
			}
			for i, st := range got {
				if st.ID != tc.wantIDs[i] {
					t.Errorf("staff %d = %q, want %q", i, st.ID, tc.wantIDs[i])
				}
			}
		})
	}
}

func TestStoreStaffCredits(t *testing.T) {
	s := mustStore(t)

	// va-one voices alpha-hero in ja (the default cast, which applies only to
	// the appearance that does not override it) and zed-friend in en (a
	// character with no appearances, so no series ids).
	credits := s.StaffCredits("va-one")
	if len(credits) != 2 {
		t.Fatalf("StaffCredits(va-one) = %d credits, want 2: %+v", len(credits), credits)
	}
	if credits[0].CharacterID != "alpha-hero" || credits[0].Language != "ja" {
		t.Errorf("credit 0 = %s/%s", credits[0].CharacterID, credits[0].Language)
	}
	if len(credits[0].SeriesIDs) != 1 || credits[0].SeriesIDs[0] != "aaa-main" {
		t.Errorf("credit 0 series = %v, want [aaa-main]", credits[0].SeriesIDs)
	}
	if credits[1].CharacterID != "zed-friend" || credits[1].Language != "en" || credits[1].SeriesIDs != nil {
		t.Errorf("credit 1 = %+v", credits[1])
	}

	// va-two is only cast through the appearance-level override.
	credits = s.StaffCredits("va-two")
	if len(credits) != 1 || credits[0].Language != "en" || credits[0].SeriesIDs[0] != "zzz" {
		t.Fatalf("StaffCredits(va-two) = %+v", credits)
	}

	if got := s.StaffCredits("nobody"); got != nil {
		t.Errorf("StaffCredits(nobody) = %v, want nil", got)
	}
}

// A duplicated (staff, language, series) triple is recorded once.
func TestStoreStaffCreditsDeduplicates(t *testing.T) {
	const dupYAML = `series:
  id: d
  characters:
    - id: dup-cast
      appearances:
        - seriesId: d
          voiceActors:
            - staffId: va
              language: ja
            - staffId: va
              language: ja
`
	s, err := NewStoreFromDataset(fstest.MapFS{
		"data/series/d.yaml": {Data: []byte(dupYAML)},
		"data/staff/s.yaml":  {Data: []byte("staff:\n  - id: va\n")},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	credits := s.StaffCredits("va")
	if len(credits) != 1 || len(credits[0].SeriesIDs) != 1 {
		t.Fatalf("StaffCredits(va) = %+v, want one credit for one series", credits)
	}
}

// A casting that names a staff member with no record of their own is dropped
// rather than indexed against an invented row. Such a credit was never
// reachable — GetStaff answers NotFound for the same id, and that is the only
// call that reads credits — and the dataset lint is what catches the dangling
// reference itself.
func TestStoreCreditsToUnknownStaffAreDropped(t *testing.T) {
	const yaml = `series:
  id: d
  characters:
    - id: dup-cast
      appearances:
        - seriesId: d
          voiceActors:
            - staffId: ghost
              language: ja
`
	s, err := NewStoreFromDataset(fstest.MapFS{"data/series/d.yaml": {Data: []byte(yaml)}})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if got := s.StaffCredits("ghost"); got != nil {
		t.Errorf("StaffCredits(ghost) = %+v, want nil", got)
	}
	if _, ok := s.Staff("ghost"); ok {
		t.Error("Staff(ghost) should not be found")
	}
}

// The index names the file each record lives in. If that file has since
// changed so that it no longer holds the record, the two have drifted — which
// must surface as an error, not as a "not found" that looks like a typo.
func TestStoreReportsIndexDriftRatherThanNotFound(t *testing.T) {
	dataset := fstest.MapFS{
		"data/series/a.yaml": {Data: []byte("franchise:\n  id: f\n  series:\n    - id: s\n  characters:\n    - id: c\n")},
	}
	ix, err := index.Build(dataset)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Same index, but every record file is now something else entirely.
	drifted := NewStore(ix, fstest.MapFS{
		"data/series/a.yaml": {Data: []byte("series:\n  id: other\n")},
	})
	if _, _, err := drifted.Franchise("f"); err == nil {
		t.Error("Franchise: want an error naming the drift")
	}
	if _, _, _, err := drifted.Series("s"); err == nil {
		t.Error("Series: want an error naming the drift")
	}
	if _, _, err := drifted.Character("c"); err == nil {
		t.Error("Character: want an error naming the drift")
	}
	if _, err := drifted.CharactersPage("", "", "", 0); err == nil {
		t.Error("CharactersPage: want an error naming the drift")
	}
	if _, err := drifted.FranchisesPage("", 0); err == nil {
		t.Error("FranchisesPage: want an error naming the drift")
	}
}

// A record file the index names but that cannot be read or parsed at all.
func TestStoreReportsUnreadableRecords(t *testing.T) {
	dataset := fstest.MapFS{"data/series/a.yaml": {Data: []byte("series:\n  id: s\n")}}
	ix, err := index.Build(dataset)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	missing := NewStore(ix, fstest.MapFS{})
	if _, _, _, err := missing.Series("s"); err == nil {
		t.Error("Series over an empty filesystem: want a read error")
	}

	corrupt := NewStore(ix, fstest.MapFS{"data/series/a.yaml": {Data: []byte("series: [unterminated")}})
	if _, _, _, err := corrupt.Series("s"); err == nil {
		t.Error("Series over a malformed record: want a parse error")
	}
}

// The record cache must hand every caller the same parse, and must not grow
// without bound as different files are read.
func TestRecordCacheIsSharedAndBounded(t *testing.T) {
	dataset := fstest.MapFS{}
	for i := 0; i < recordCacheSize*2; i++ {
		name := fmt.Sprintf("data/series/s%02d.yaml", i)
		dataset[name] = &fstest.MapFile{Data: []byte(fmt.Sprintf("series:\n  id: s%02d\n", i))}
	}
	s, err := NewStoreFromDataset(dataset)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	first, _, _, err := s.Series("s00")
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	again, _, _, err := s.Series("s00")
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if first != again {
		t.Error("a cached record was parsed twice")
	}

	for i := 0; i < recordCacheSize*2; i++ {
		if _, _, _, err := s.Series(fmt.Sprintf("s%02d", i)); err != nil {
			t.Fatalf("Series(s%02d): %v", i, err)
		}
	}
	s.mu.Lock()
	size := len(s.cache)
	s.mu.Unlock()
	if size > recordCacheSize {
		t.Errorf("cache holds %d records, want at most %d", size, recordCacheSize)
	}
}
