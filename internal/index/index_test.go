package index

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// The five bytes that carry structure in the format, plus the values that break
// naive implementations: an escape at the very end, an escape before an escape,
// and text that is nothing but separators.
var escapeCases = []string{
	"",
	"plain",
	"tab\there",
	"newline\nhere",
	"pipe|here",
	"equals=here",
	"back\\slash",
	"\\",
	"\\\\",
	"|=|=",
	"\t\n|=\\",
	"フェイト",
	"Fate/stay night [Unlimited Blade Works]",
	"a\\tb", // a literal backslash-t, which must not decode to a tab
}

func TestEscapeRoundTrip(t *testing.T) {
	for _, in := range escapeCases {
		enc := escape(in)
		if strings.ContainsAny(enc, "\t\n|=") {
			t.Errorf("escape(%q) = %q, which still contains a separator", in, enc)
		}
		if got := unescape(enc); got != in {
			t.Errorf("unescape(escape(%q)) = %q", in, got)
		}
	}
}

// An escaped value must survive being written into a row and read back out by
// column, which is the property the round-trip above does not by itself prove.
func TestEscapedValuesSurviveAColumn(t *testing.T) {
	row := strings.Join([]string{escape("a\tb"), escape("c|d"), escape("e=f")}, "\t")
	c, err := cols(row, 0, 3)
	if err != nil {
		t.Fatalf("cols: %v", err)
	}
	want := []string{"a\tb", "c|d", "e=f"}
	for i, w := range want {
		if got := unescape(c[i].text(row)); got != w {
			t.Errorf("column %d = %q, want %q", i, got, w)
		}
	}
}

func TestUnescapeLeavesUnknownSequencesAlone(t *testing.T) {
	// A byte the writer never emits. Serving it verbatim beats refusing to
	// boot over one malformed title.
	if got := unescape(`a\qb`); got != `a\qb` {
		t.Errorf("unescape = %q, want the input unchanged", got)
	}
}

func TestTitleRoundTrip(t *testing.T) {
	in := model.Title{
		Original:     "銀魂|first",
		Translations: map[string]string{"en": "Gintama=the movie", "ja-Latn": "Gintama"},
	}
	got := unpackTitle(packTitle(in))
	if got.Original != in.Original {
		t.Errorf("Original = %q, want %q", got.Original, in.Original)
	}
	for lang, want := range in.Translations {
		if got.Translations[lang] != want {
			t.Errorf("Translations[%q] = %q, want %q", lang, got.Translations[lang], want)
		}
	}
}

func TestPackTitleIsStable(t *testing.T) {
	// Map iteration order must not reach the file, or every regeneration would
	// produce a spurious diff.
	in := model.Title{Translations: map[string]string{"ja": "あ", "en": "a", "ko": "ㅂ", "fr": "e"}}
	first := packTitle(in)
	for i := 0; i < 50; i++ {
		if got := packTitle(in); got != first {
			t.Fatalf("packTitle is not deterministic: %q then %q", first, got)
		}
	}
}

func TestUnpackTitleIgnoresMalformedElements(t *testing.T) {
	// An element with no "=" is not a translation; it must not become one.
	got := unpackTitle("orig|nonsense|en=Real")
	if got.Original != "orig" || got.Translations["en"] != "Real" || len(got.Translations) != 1 {
		t.Errorf("unpackTitle = %+v", got)
	}
}

func TestTitleMatches(t *testing.T) {
	packed := packTitle(model.Title{
		Original:     "鋼の錬金術師",
		Translations: map[string]string{"en": "Fullmetal Alchemist", "ja": "ハガレン"},
	})
	tests := []struct {
		query string
		want  bool
	}{
		{"fullmetal", true},
		{"ALCHEMIST", true},
		{"錬金術", true},
		{"ハガレン", true},
		{"nothing", false},
		// A language tag is a key of the packing, not part of any title. If it
		// matched, every Japanese title would answer to the query "ja".
		{"ja", false},
		{"en", false},
		// Nor may the packing's own separators match.
		{"|", false},
		{"=", false},
		{"en=", false},
	}
	for _, tc := range tests {
		if got := titleMatches(packed, tc.query); got != tc.want {
			t.Errorf("titleMatches(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestContainsFold(t *testing.T) {
	tests := []struct {
		haystack, needle string
		want             bool
	}{
		{"Fullmetal", "full", true},
		{"FULLMETAL", "metal", true},
		{"Fullmetal", "", true},
		{"", "x", false},
		{"short", "much longer needle", false},
		{"ÉCLAIR", "éclair", true},
		{"日本語テキスト", "テキス", true},
		{"abc", "abcd", false},
		// The needle must be found at a rune boundary, not inside one.
		{"日本", "\x9c", false},
	}
	for _, tc := range tests {
		if got := containsFold(tc.haystack, tc.needle); got != tc.want {
			t.Errorf("containsFold(%q, %q) = %v, want %v", tc.haystack, tc.needle, got, tc.want)
		}
	}
}

// --- Reader validation -----------------------------------------------------

func TestOpenRejectsMalformedFiles(t *testing.T) {
	tests := []struct {
		name, blob, want string
	}{
		{"empty", "", "empty"},
		{"wrong magic", "!something-else\t1\n", "not an index file"},
		{"no version", "!anime-metadata-db-index\n", "not an index file"},
		{"unreadable version", "!anime-metadata-db-index\tabc\n", "unreadable version"},
		{"future version", "!anime-metadata-db-index\t99\n", "version 99"},
		{"row before a section", "!anime-metadata-db-index\t1\nstray\n", "row before any section"},
		{"malformed stat", "!anime-metadata-db-index\t1\n!stats\tnonsense\n", "malformed stat"},
		{"non-numeric stat", "!anime-metadata-db-index\t1\n!stats\tseries=many\n", "stat series"},
		{
			"wrong column count",
			"!anime-metadata-db-index\t1\n!franchises\tid\tfile\ttitles\nonly\ttwo\n",
			"want 3",
		},
		{
			"series naming a franchise that does not exist",
			"!anime-metadata-db-index\t1\n!series\tid\tfile\tfranchise\ttitles\ns\ts.yaml\t7\t\n",
			"out of range",
		},
		{
			"series with an unreadable franchise column",
			"!anime-metadata-db-index\t1\n!series\tid\tfile\tfranchise\ttitles\ns\ts.yaml\tx\t\n",
			"franchise",
		},
		{
			"entry of an unknown kind",
			"!anime-metadata-db-index\t1\n!entries\tkind\trow\ta\tb\tc\td\nmovie\t1\t\t\t\t\n",
			"unknown entry kind",
		},
		{
			"entry pointing past the series it names",
			"!anime-metadata-db-index\t1\n!entries\tkind\trow\ta\tb\tc\td\nseries\t4\t\t\t\t\n",
			"out of range",
		},
		{
			"work of an unknown kind",
			"!anime-metadata-db-index\t1\n!works\ta\tb\tc\td\te\tf\tg\th\ti\tj\tk\tl\tm\tn\to\n" +
				"opera\tw\t1\t\t\t\t\t\t\t\t\t\t\t\t\n",
			"unknown work kind",
		},
		{
			"work naming a series that does not exist",
			"!anime-metadata-db-index\t1\n!works\ta\tb\tc\td\te\tf\tg\th\ti\tj\tk\tl\tm\tn\to\n" +
				"season\tw\t9\t\t\t\t\t\t\t\t\t\t\t\t\n",
			"series row 9 out of range",
		},
		{
			"credit naming a staff member that does not exist",
			"!anime-metadata-db-index\t1\n!credits\tstaff\tcharacter\tlanguage\tseries\n3\t1\tja\t\n",
			"staff row 3 out of range",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Open(tc.blob)
			if err == nil {
				t.Fatalf("Open(%q) succeeded, want an error mentioning %q", tc.blob, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Open error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A section this reader does not know is data a newer writer added. Skipping it
// keeps an old server serving a new index instead of refusing to boot.
func TestOpenSkipsUnknownSections(t *testing.T) {
	blob := "!anime-metadata-db-index\t1\n!stats\tseries=1\n!something-new\ta\tb\nx\ty\n"
	ix, err := Open(blob)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if ix.Stats().Series != 1 {
		t.Errorf("Stats().Series = %d, want 1", ix.Stats().Series)
	}
}

// --- Build and query -------------------------------------------------------

// fixture is a dataset small enough to reason about and shaped like the real
// one: a franchise with two series, a standalone series, cast at both levels,
// and a character appearing in more than one series.
func fixture() fstest.MapFS {
	return fstest.MapFS{
		"data/series/aaa.yaml": {Data: []byte(`franchise:
  id: aaa
  titles:
    original: エーエーエー
    translations:
      en: Triple A
  series:
    - id: aaa-main
      titles:
        translations:
          en: Triple A
      seasons:
        - id: aaa-s1
          number: 1
          releaseYear: 2019
          releaseSeason: SPRING
          episodes:
            - airedNumber: 1
            - airedNumber: 2
        - id: aaa-s2
          number: 2
          releaseYear: 2021
          releaseSeason: FALL
          releaseDate: 2021-10-03
          episodes:
            - airedNumber: 1
      movies:
        - id: aaa-movie
          releaseYear: 2020
          titles:
            translations:
              en: Triple A The Movie
    - id: aaa-side
      titles:
        translations:
          en: Triple A Side Story
      specials:
        - id: aaa-ova
          format: OVA
          releaseYear: 2022
  characters:
    - id: alpha
      names:
        original: アルファ
        translations:
          en: Alpha
      appearances:
        - seriesId: aaa-main
          voiceActors:
            - staffId: va-one
              language: ja
        - seriesId: zzz
      voiceActors:
        - staffId: va-one
          language: ja
`)},
		"data/series/zzz.yaml": {Data: []byte(`series:
  id: zzz
  titles:
    translations:
      en: Zed
  seasons:
    - id: zzz-s1
      number: 1
      releaseYear: 2019
      releaseSeason: SPRING
  characters:
    - id: zed
      names:
        translations:
          en: Zed
      appearances:
        - seriesId: zzz
          voiceActors:
            - staffId: va-two
              language: en
`)},
		"data/staff/s.yaml": {Data: []byte(`staff:
  - id: va-one
    names:
      original: 一
      translations:
        en: VA One
    externalIds:
      anilistId: 11
      wikidataId: Q11
  - id: va-two
    names:
      translations:
        en: VA Two
`)},
	}
}

func mustIndex(t *testing.T) *Index {
	t.Helper()
	ix, err := Build(fixture())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return ix
}

// The generated file has to be byte-identical between runs, or `make index`
// would produce a diff every time it ran and the CI check would be noise.
func TestWriteIsDeterministic(t *testing.T) {
	var first bytes.Buffer
	d, err := Load(fixture())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := d.WriteTo(&first); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	for i := 0; i < 20; i++ {
		d, err := Load(fixture())
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		var next bytes.Buffer
		if _, err := d.WriteTo(&next); err != nil {
			t.Fatalf("WriteTo: %v", err)
		}
		if next.String() != first.String() {
			t.Fatal("WriteTo produced different bytes for the same dataset")
		}
	}
}

func TestStats(t *testing.T) {
	got := mustIndex(t).Stats()
	want := Stats{
		Franchises: 1, Series: 3, Seasons: 3, Episodes: 3,
		Characters: 2, Staff: 2,
		EarliestReleaseYear: 2019, LatestReleaseYear: 2022,
	}
	if got != want {
		t.Errorf("Stats() = %+v, want %+v", got, want)
	}
}

func TestCatalogCarriesAggregates(t *testing.T) {
	ix := mustIndex(t)
	page, err := ix.Catalog(nil, "", 0)
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	// The franchise, then its two series, then the standalone series.
	wantIDs := []string{"aaa", "aaa-main", "aaa-side", "zzz"}
	if len(page.Items) != len(wantIDs) {
		t.Fatalf("Catalog returned %d entries, want %d", len(page.Items), len(wantIDs))
	}
	for i, want := range wantIDs {
		if page.Items[i].ID != want {
			t.Errorf("entry %d = %q, want %q", i, page.Items[i].ID, want)
		}
	}
	// The franchise rolls up both of its series.
	f := page.Items[0]
	if f.Works != 4 || f.Episodes != 3 || f.FirstReleaseYear != 2019 || f.LatestReleaseYear != 2022 {
		t.Errorf("franchise aggregate = %+v", f)
	}
	// A series under a franchise names its owner; a standalone one does not.
	if page.Items[1].FranchiseID != "aaa" {
		t.Errorf("aaa-main.FranchiseID = %q, want aaa", page.Items[1].FranchiseID)
	}
	if page.Items[3].FranchiseID != "" {
		t.Errorf("zzz.FranchiseID = %q, want empty", page.Items[3].FranchiseID)
	}
}

func TestCatalogKindFilter(t *testing.T) {
	ix := mustIndex(t)
	kind := EntryFranchise
	page, err := ix.Catalog(&kind, "", 0)
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "aaa" || page.Total != 1 {
		t.Errorf("franchise-only catalog = %+v", page)
	}
}

func TestWorksCarryTheirSeriesAndDates(t *testing.T) {
	ix := mustIndex(t)
	page, err := ix.Works(WorkFilter{SeriesID: "aaa-main"}, "", 0)
	if err != nil {
		t.Fatalf("Works: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("Works(aaa-main) returned %d, want 3", len(page.Items))
	}
	s2 := page.Items[1]
	if s2.ID != "aaa-s2" || s2.Number != 2 || s2.ReleaseSeason != model.SeasonFall {
		t.Errorf("second work = %+v", s2)
	}
	if s2.ReleaseDate == nil || s2.ReleaseDate.Format("2006-01-02") != "2021-10-03" {
		t.Errorf("release date = %v, want 2021-10-03", s2.ReleaseDate)
	}
	// A work names its series so a row can render without a second lookup.
	if s2.SeriesID != "aaa-main" || s2.SeriesTitles.Translations["en"] != "Triple A" {
		t.Errorf("series on work = %q / %+v", s2.SeriesID, s2.SeriesTitles)
	}
	// A season with no date of its own keeps a nil date rather than a zero one.
	if page.Items[0].ReleaseDate != nil {
		t.Errorf("undated season got date %v", page.Items[0].ReleaseDate)
	}
}

func TestWorksFilters(t *testing.T) {
	ix := mustIndex(t)
	special := WorkSpecial
	tests := []struct {
		name    string
		filter  WorkFilter
		wantIDs []string
	}{
		{"everything", WorkFilter{}, []string{"aaa-s1", "aaa-s2", "aaa-movie", "aaa-ova", "zzz-s1"}},
		{"by year", WorkFilter{ReleaseYear: 2019}, []string{"aaa-s1", "zzz-s1"}},
		{"by quarter", WorkFilter{ReleaseSeason: model.SeasonFall}, []string{"aaa-s2"}},
		{"by kind", WorkFilter{Kind: &special}, []string{"aaa-ova"}},
		{"by series", WorkFilter{SeriesID: "aaa-side"}, []string{"aaa-ova"}},
		{"unknown series", WorkFilter{SeriesID: "nope"}, nil},
		// A work with no title of its own is still found through its series.
		{"query matches the series title", WorkFilter{Query: "zed"}, []string{"zzz-s1"}},
		{"query matches the work title", WorkFilter{Query: "the movie"}, []string{"aaa-movie"}},
		{"query matches nothing", WorkFilter{Query: "absent"}, nil},
		{"year and quarter together", WorkFilter{ReleaseYear: 2019, ReleaseSeason: model.SeasonFall}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err := ix.Works(tc.filter, "", 0)
			if err != nil {
				t.Fatalf("Works: %v", err)
			}
			var got []string
			for _, w := range page.Items {
				got = append(got, w.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("Works(%+v) = %v, want %v", tc.filter, got, tc.wantIDs)
			}
			if page.Total != len(tc.wantIDs) {
				t.Errorf("Total = %d, want %d", page.Total, len(tc.wantIDs))
			}
		})
	}
}

func TestSearch(t *testing.T) {
	ix := mustIndex(t)
	tests := []struct {
		query   string
		wantIDs []string
	}{
		{"triple a", []string{"aaa", "aaa-main", "aaa-side"}},
		{"エーエーエー", []string{"aaa"}},
		{"zed", []string{"zzz"}},
		{"side story", []string{"aaa-side"}},
		{"", nil},
		{"   ", nil},
		{"nothing here", nil},
	}
	for _, tc := range tests {
		page, err := ix.Search(tc.query, "", 0)
		if err != nil {
			t.Fatalf("Search(%q): %v", tc.query, err)
		}
		var got []string
		for _, e := range page.Items {
			got = append(got, e.ID)
		}
		if strings.Join(got, ",") != strings.Join(tc.wantIDs, ",") {
			t.Errorf("Search(%q) = %v, want %v", tc.query, got, tc.wantIDs)
		}
	}
}

func TestCharactersAreFiledUnderEverySeriesTheyAppearIn(t *testing.T) {
	ix := mustIndex(t)
	tests := []struct {
		name     string
		seriesID string
		query    string
		wantIDs  []string
	}{
		{"whole cast", "", "", []string{"alpha", "zed"}},
		{"owning series", "aaa-main", "", []string{"alpha"}},
		// alpha appears in zzz, which lives in another record entirely.
		{"cross-record appearance", "zzz", "", []string{"alpha", "zed"}},
		{"unknown series", "nope", "", nil},
		{"by name", "", "alpha", []string{"alpha"}},
		{"by native name", "", "アルファ", []string{"alpha"}},
		{"name matching nothing", "", "nobody", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err := ix.Characters(tc.seriesID, tc.query, "", 0)
			if err != nil {
				t.Fatalf("Characters: %v", err)
			}
			var got []string
			for _, r := range page.Items {
				got = append(got, r.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("Characters(%q, %q) = %v, want %v", tc.seriesID, tc.query, got, tc.wantIDs)
			}
		})
	}
}

func TestStaffPageAndDetail(t *testing.T) {
	ix := mustIndex(t)
	tests := []struct {
		name     string
		language string
		query    string
		wantIDs  []string
	}{
		{"everyone", "", "", []string{"va-one", "va-two"}},
		{"by language", "ja", "", []string{"va-one"}},
		{"by other language", "en", "", []string{"va-two"}},
		{"language nobody has", "fr", "", nil},
		{"by name", "", "va two", []string{"va-two"}},
		{"by native name", "", "一", []string{"va-one"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			page, err := ix.StaffPage(tc.language, tc.query, "", 0)
			if err != nil {
				t.Fatalf("StaffPage: %v", err)
			}
			var got []string
			for _, st := range page.Items {
				got = append(got, st.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("StaffPage(%q, %q) = %v, want %v", tc.language, tc.query, got, tc.wantIDs)
			}
		})
	}

	// Staff records live in the index in full, external ids included, so a
	// staff response never reopens a file.
	st, ok := ix.Staff("va-one")
	if !ok {
		t.Fatal("Staff(va-one) not found")
	}
	if st.Names.Original != "一" || st.ExternalIDs.AnilistID != 11 || st.ExternalIDs.WikidataID != "Q11" {
		t.Errorf("Staff(va-one) = %+v", st)
	}
	if _, ok := ix.Staff("nobody"); ok {
		t.Error("Staff(nobody) should not be found")
	}
}

func TestCredits(t *testing.T) {
	ix := mustIndex(t)
	credits := ix.Credits("va-one")
	if len(credits) != 1 {
		t.Fatalf("Credits(va-one) = %+v, want one", credits)
	}
	c := credits[0]
	if c.CharacterID != "alpha" || c.Language != "ja" {
		t.Errorf("credit = %+v", c)
	}
	// A credit carries the character's names so a staff page renders without
	// reading a record file.
	if c.CharacterNames.Translations["en"] != "Alpha" {
		t.Errorf("credit names = %+v", c.CharacterNames)
	}
	// The default cast applies to the appearance that does not override it, so
	// the credit covers both series.
	if strings.Join(c.SeriesIDs, ",") != "aaa-main,zzz" {
		t.Errorf("credit series = %v, want aaa-main,zzz", c.SeriesIDs)
	}
	if got := ix.Credits("nobody"); got != nil {
		t.Errorf("Credits(nobody) = %v, want nil", got)
	}
}

func TestLookupsNameTheirRecordFile(t *testing.T) {
	ix := mustIndex(t)
	if file, ok := ix.Franchise("aaa"); !ok || file != "aaa.yaml" {
		t.Errorf("Franchise(aaa) = %q, %v", file, ok)
	}
	if _, ok := ix.Franchise("zzz"); ok {
		t.Error("a series id is not a franchise id")
	}
	file, franchiseID, ok := ix.Series("aaa-side")
	if !ok || file != "aaa.yaml" || franchiseID != "aaa" {
		t.Errorf("Series(aaa-side) = %q, %q, %v", file, franchiseID, ok)
	}
	if _, franchiseID, _ := ix.Series("zzz"); franchiseID != "" {
		t.Errorf("standalone series reported franchise %q", franchiseID)
	}
	if _, _, ok := ix.Series("missing"); ok {
		t.Error("Series(missing) should not be found")
	}
	if file, ok := ix.Character("zed"); !ok || file != "zzz.yaml" {
		t.Errorf("Character(zed) = %q, %v", file, ok)
	}
	if _, ok := ix.Character("missing"); ok {
		t.Error("Character(missing) should not be found")
	}
	titles, ok := ix.SeriesTitle("zzz")
	if !ok || titles.Translations["en"] != "Zed" {
		t.Errorf("SeriesTitle(zzz) = %+v, %v", titles, ok)
	}
	if _, ok := ix.SeriesTitle("missing"); ok {
		t.Error("SeriesTitle(missing) should not be found")
	}
	refs := ix.Franchises()
	if len(refs) != 1 || refs[0].ID != "aaa" || refs[0].File != "aaa.yaml" {
		t.Errorf("Franchises() = %+v", refs)
	}
}

func TestBuildRejectsBrokenDatasets(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
		want string
	}{
		{"missing data dir", fstest.MapFS{"other.txt": {Data: []byte("x")}}, "read dataset dir"},
		{
			"malformed yaml",
			fstest.MapFS{"data/series/bad.yaml": {Data: []byte("franchise: [unterminated")}},
			"parse",
		},
		{
			"record that is neither",
			fstest.MapFS{"data/series/empty.yaml": {Data: []byte("{}")}},
			"neither franchise nor series",
		},
		{
			"duplicate franchise",
			fstest.MapFS{
				"data/series/a.yaml": {Data: []byte("franchise:\n  id: dup\n  series: []\n")},
				"data/series/b.yaml": {Data: []byte("franchise:\n  id: dup\n  series: []\n")},
			},
			"duplicate franchise id",
		},
		{
			"duplicate series",
			fstest.MapFS{
				"data/series/a.yaml": {Data: []byte("series:\n  id: dup\n")},
				"data/series/b.yaml": {Data: []byte("series:\n  id: dup\n")},
			},
			"duplicate series id",
		},
		{
			"duplicate series nested in a franchise",
			fstest.MapFS{"data/series/a.yaml": {Data: []byte(
				"franchise:\n  id: f\n  series:\n    - id: dup\n    - id: dup\n")}},
			"duplicate series id",
		},
		{
			"duplicate character across records",
			fstest.MapFS{
				"data/series/a.yaml": {Data: []byte("series:\n  id: a\n  characters:\n    - id: dup\n")},
				"data/series/b.yaml": {Data: []byte("series:\n  id: b\n  characters:\n    - id: dup\n")},
			},
			"duplicate character id",
		},
		{
			"malformed staff yaml",
			fstest.MapFS{
				"data/series/a.yaml": {Data: []byte("series:\n  id: a\n")},
				"data/staff/s.yaml":  {Data: []byte("staff: [unterminated")},
			},
			"parse",
		},
		{
			"duplicate staff id",
			fstest.MapFS{
				"data/series/a.yaml": {Data: []byte("series:\n  id: a\n")},
				"data/staff/s.yaml":  {Data: []byte("staff:\n  - id: dup\n  - id: dup\n")},
			},
			"duplicate staff id",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(tc.fsys)
			if err == nil {
				t.Fatalf("Build succeeded, want an error mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Build error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A dataset with no staff subtree loads: staff are optional.
func TestBuildWithoutStaff(t *testing.T) {
	ix, err := Build(fstest.MapFS{"data/series/a.yaml": {Data: []byte("series:\n  id: a\n")}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ix.Stats().Staff != 0 || ix.Stats().Series != 1 {
		t.Errorf("Stats() = %+v", ix.Stats())
	}
}

// Non-YAML files and subdirectories in the data tree are ignored rather than
// treated as records.
func TestLoadIgnoresNonYAML(t *testing.T) {
	ix, err := Build(fstest.MapFS{
		"data/series/a.yaml":      {Data: []byte("series:\n  id: a\n")},
		"data/series/README.md":   {Data: []byte("not a record")},
		"data/series/nested/x.go": {Data: []byte("package x")},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ix.Stats().Series != 1 {
		t.Errorf("Stats().Series = %d, want 1", ix.Stats().Series)
	}
}

// A character cast to a staff member with no record of their own is skipped:
// the row has no staff to reference. The dataset lint is what reports the
// dangling id itself.
func TestCreditsToUnknownStaffAreSkipped(t *testing.T) {
	ix, err := Build(fstest.MapFS{"data/series/a.yaml": {Data: []byte(`series:
  id: a
  characters:
    - id: c
      voiceActors:
        - staffId: ghost
          language: ja
`)}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := ix.Credits("ghost"); got != nil {
		t.Errorf("Credits(ghost) = %+v, want nil", got)
	}
}

// A character with no appearances still records its default cast, against no
// particular series.
func TestCreditWithoutAppearances(t *testing.T) {
	ix, err := Build(fstest.MapFS{
		"data/series/a.yaml": {Data: []byte(`series:
  id: a
  characters:
    - id: c
      voiceActors:
        - staffId: va
          language: ja
`)},
		"data/staff/s.yaml": {Data: []byte("staff:\n  - id: va\n")},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	credits := ix.Credits("va")
	if len(credits) != 1 || credits[0].CharacterID != "c" || len(credits[0].SeriesIDs) != 0 {
		t.Errorf("Credits(va) = %+v", credits)
	}
}

func TestPagingCoversEverythingExactlyOnce(t *testing.T) {
	ix := mustIndex(t)
	seen := map[string]int{}
	token := ""
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("paging did not terminate")
		}
		page, err := ix.Works(WorkFilter{}, token, 2)
		if err != nil {
			t.Fatalf("Works: %v", err)
		}
		for _, w := range page.Items {
			seen[w.ID]++
		}
		if page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	if len(seen) != 5 {
		t.Errorf("paged over %d works, want 5", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("work %q appeared %d times", id, n)
		}
	}
}

func TestQueriesRejectBadPageTokens(t *testing.T) {
	ix := mustIndex(t)
	const bad = "!!!not-base64!!!"
	if _, err := ix.Catalog(nil, bad, 0); err == nil {
		t.Error("Catalog accepted a malformed token")
	}
	if _, err := ix.Works(WorkFilter{}, bad, 0); err == nil {
		t.Error("Works accepted a malformed token")
	}
	if _, err := ix.Search("triple", bad, 0); err == nil {
		t.Error("Search accepted a malformed token")
	}
	if _, err := ix.Characters("", "", bad, 0); err == nil {
		t.Error("Characters accepted a malformed token")
	}
	if _, err := ix.StaffPage("", "", bad, 0); err == nil {
		t.Error("StaffPage accepted a malformed token")
	}
}
