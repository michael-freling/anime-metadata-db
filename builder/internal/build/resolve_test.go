package build

import (
	"sort"
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// A series and its installments as upstream carries them: each entry lists the
// series' native title among its synonyms, which is the whole basis of the
// resolution.
const familyDB = `{"data":[
 {"sources":["https://anilist.co/anime/100"],"title":"Show","type":"TV","episodes":12,
  "animeSeason":{"season":"FALL","year":2020},"synonyms":["ショー"]},
 {"sources":["https://anilist.co/anime/200"],"title":"Show 2nd Season","type":"TV","episodes":12,
  "animeSeason":{"season":"SPRING","year":2022},"synonyms":["ショー"]},
 {"sources":["https://anilist.co/anime/300"],"title":"Show: The Movie","type":"MOVIE","episodes":1,
  "animeSeason":{"season":"SUMMER","year":2023},"synonyms":["ショー"]},
 {"sources":["https://anilist.co/anime/400"],"title":"Show OVA","type":"OVA","episodes":2,
  "animeSeason":{"season":"WINTER","year":2021},"synonyms":["ショー"]},
 {"sources":["https://anilist.co/anime/999"],"title":"Something Else","type":"TV","episodes":12,
  "animeSeason":{"season":"FALL","year":1999}}
]}`

func showSeries() *model.Series {
	return &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Seasons: []model.Season{
			{ID: "show-s1", Number: 1},
			{ID: "show-s2", Number: 2},
		},
		Movies:   []model.Movie{{ID: "show-film"}},
		Specials: []model.Special{{ID: "show-ova"}},
	}
}

// The point of the whole thing: no id is authored, and every installment gets
// the right one — seasons in airing order, and each kind drawn only from the
// upstream media types that can fill it.
func TestResolveInstallmentsAssignsEveryKind(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := showSeries()
	report := &Report{}
	b.resolveInstallments(s, report)

	for _, tc := range []struct {
		what string
		got  int
		want int
	}{
		{"season 1", s.Seasons[0].ExternalIDs.AnilistID, 100},
		{"season 2", s.Seasons[1].ExternalIDs.AnilistID, 200},
		{"movie", s.Movies[0].ExternalIDs.AnilistID, 300},
		{"special", s.Specials[0].ExternalIDs.AnilistID, 400},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.what, tc.got, tc.want)
		}
	}
	if report.Coverage.Derived != 4 {
		t.Errorf("Derived = %d, want 4", report.Coverage.Derived)
	}
	if !report.Empty() {
		t.Errorf("a clean resolution says nothing: %v", report.Notes)
	}
}

// Seasons are dealt out by airing date, not by the order they appear in the
// override — an override listing season 2 first must still get 2's id.
func TestResolveInstallmentsOrdersByAiringDate(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Seasons: []model.Season{
			{ID: "show-s2", Number: 2},
			{ID: "show-s1", Number: 1},
		},
	}
	b.resolveInstallments(s, &Report{})

	if s.Seasons[0].ExternalIDs.AnilistID != 200 || s.Seasons[1].ExternalIDs.AnilistID != 100 {
		t.Errorf("ids follow the season number, not the file order: %+v", s.Seasons)
	}
}

// An authored id is never overwritten: it is the anchor the build reads every
// other fact through, and a resolution that disagrees is reported rather than
// applied. Which of the two is wrong is not something the build can know.
func TestResolveInstallmentsNeverOverwritesAnAuthoredID(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := showSeries()
	s.Seasons[0].ExternalIDs.AnilistID = 999 // authored, and deliberately wrong
	report := &Report{}
	b.resolveInstallments(s, report)

	if s.Seasons[0].ExternalIDs.AnilistID != 999 {
		t.Error("an authored id was overwritten")
	}
	got := report.String()
	if !strings.Contains(got, "season show-s1") || !strings.Contains(got, "resolves to anilistId 100 from the series' title, but 999 is authored") {
		t.Errorf("the disagreement must name both ids:\n%s", got)
	}
	// The rest of the kind still resolves: an unauthored slot has no anchor to
	// preserve, and the resolution is the only answer there is.
	if s.Seasons[1].ExternalIDs.AnilistID != 200 {
		t.Errorf("season 2 = %d, want 200", s.Seasons[1].ExternalIDs.AnilistID)
	}
}

// The common case now the ids are authored: both routes reach the same entry,
// which is the strongest corroboration available and worth counting.
func TestResolveInstallmentsCountsAgreement(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := showSeries()
	s.Seasons[0].ExternalIDs.AnilistID = 100
	s.Seasons[1].ExternalIDs.AnilistID = 200
	report := &Report{}
	b.resolveInstallments(s, report)

	if report.Coverage.Agreed != 2 {
		t.Errorf("Agreed = %d, want 2", report.Coverage.Agreed)
	}
	if !report.Empty() {
		t.Errorf("agreement is not a finding: %v", report.Notes)
	}
}

// The safety property the measurements turned on: when the counts disagree,
// nothing is assigned and nothing is claimed. A spare candidate would have to
// be attributed to some installment, and a wrong answer there would assert that
// a season holds another's id.
func TestResolveInstallmentsRefusesWhenCountsDisagree(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Seasons: []model.Season{
			{ID: "show-s1", Number: 1},
			{ID: "show-s2", Number: 2},
			{ID: "show-s3", Number: 3}, // upstream carries only two
		},
	}
	report := &Report{}
	b.resolveInstallments(s, report)

	for i, season := range s.Seasons {
		if season.ExternalIDs.AnilistID != 0 {
			t.Errorf("season %d was assigned %d despite a count mismatch", i+1, season.ExternalIDs.AnilistID)
		}
	}
	// Silent, not reported: upstream carries spin-offs and shorts under the same
	// title for many series, so a line each would say only that the title is
	// popular.
	if !report.Empty() {
		t.Errorf("a count that does not line up is not a finding: %v", report.Notes)
	}
	if report.Coverage.Derived != 0 || report.Coverage.Agreed != 0 {
		t.Errorf("nothing was resolved, so nothing should be counted: %+v", report.Coverage)
	}
}

// A title matching nothing upstream resolves nothing and says nothing here: the
// series simply keeps its authored ids, and `lookup` is what objects if it has
// none.
func TestResolveInstallmentsUnknownTitle(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := &model.Series{
		ID:      "nope",
		Titles:  model.Title{Original: "知らない題"},
		Seasons: []model.Season{{ID: "nope-s1", Number: 1}},
	}
	report := &Report{}
	b.resolveInstallments(s, report)

	if s.Seasons[0].ExternalIDs.AnilistID != 0 {
		t.Error("an unknown title must resolve nothing")
	}
	if !report.Empty() {
		t.Errorf("nothing was found, so there is nothing to report here: %v", report.Notes)
	}
}

// The romanization is a second key, because upstream indexes some series under
// a Latin-script name and others under the native one.
func TestResolveInstallmentsMatchesOnTheRomanization(t *testing.T) {
	const romanizedDB = `{"data":[
	 {"sources":["https://anilist.co/anime/100"],"title":"Show","type":"TV","episodes":12,
	  "animeSeason":{"season":"FALL","year":2020}}
	]}`
	b := New(Sources{Offline: offlineFrom(t, romanizedDB)})
	s := &model.Series{
		ID: "show",
		Titles: model.Title{
			Original:     "ショー",
			Translations: map[string]string{"ja-Latn": "Show"},
		},
		Seasons: []model.Season{{ID: "show-s1", Number: 1}},
	}
	b.resolveInstallments(s, &Report{})

	if s.Seasons[0].ExternalIDs.AnilistID != 100 {
		t.Errorf("the romanization must be tried too, got %d", s.Seasons[0].ExternalIDs.AnilistID)
	}
}

// The build must run with no offline database rather than panic on a nil index.
func TestResolveInstallmentsWithoutOfflineDatabase(t *testing.T) {
	b := New(Sources{})
	s := showSeries()
	b.resolveInstallments(s, &Report{})

	if s.Seasons[0].ExternalIDs.AnilistID != 0 {
		t.Error("no source means no resolution")
	}
}

// Split cours share a season number, so the tie is broken on the id to keep the
// order fixed — two runs over one override must agree, or data/ churns.
func TestResolveInstallmentsIsDeterministicForSplitCours(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	first, second := 0, 0
	for run := range 2 {
		one, two := 1, 2
		s := &model.Series{
			ID:     "show",
			Titles: model.Title{Original: "ショー"},
			Seasons: []model.Season{
				{ID: "show-s1p2", Number: 1, Part: &two},
				{ID: "show-s1p1", Number: 1, Part: &one},
			},
		}
		b.resolveInstallments(s, &Report{})
		if run == 0 {
			first, second = s.Seasons[0].ExternalIDs.AnilistID, s.Seasons[1].ExternalIDs.AnilistID
			continue
		}
		if s.Seasons[0].ExternalIDs.AnilistID != first || s.Seasons[1].ExternalIDs.AnilistID != second {
			t.Error("two runs over the same override disagreed")
		}
	}
	// p1 sorts before p2 on the id, so it takes the earlier-airing entry.
	if first != 200 || second != 100 {
		t.Errorf("split cours resolved to %d, %d", first, second)
	}
}

// Movies and specials carry no number, so the override's order is the author's
// and the candidates' is upstream's airing order. Pairing two of them lines up
// two lists that agree only by luck, so more than one is left alone.
func TestResolveInstallmentsSkipsUnorderedKindsWithMoreThanOne(t *testing.T) {
	const twoFilms = `{"data":[
	 {"sources":["https://anilist.co/anime/300"],"title":"A","type":"MOVIE","episodes":1,
	  "animeSeason":{"season":"SUMMER","year":2023},"synonyms":["ショー"]},
	 {"sources":["https://anilist.co/anime/301"],"title":"B","type":"MOVIE","episodes":1,
	  "animeSeason":{"season":"SUMMER","year":2024},"synonyms":["ショー"]}
	]}`
	b := New(Sources{Offline: offlineFrom(t, twoFilms)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Movies: []model.Movie{{ID: "show-film-b"}, {ID: "show-film-a"}},
	}
	report := &Report{}
	b.resolveInstallments(s, report)

	for _, m := range s.Movies {
		if m.ExternalIDs.AnilistID != 0 {
			t.Errorf("%s was paired on file order alone: %d", m.ID, m.ExternalIDs.AnilistID)
		}
	}
	if !report.Empty() {
		t.Errorf("declining to guess is not a finding: %v", report.Notes)
	}
}

// One of an unordered kind has nothing to line up wrongly, so it still resolves.
func TestResolveInstallmentsStillHandlesASingleMovie(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Movies: []model.Movie{{ID: "show-film"}},
	}
	b.resolveInstallments(s, &Report{})

	if s.Movies[0].ExternalIDs.AnilistID != 300 {
		t.Errorf("movie = %d, want 300", s.Movies[0].ExternalIDs.AnilistID)
	}
}

// A split cour sits in a later slot than its own number, so a disagreement is
// named by the node rather than by its position in the ordering.
func TestResolveInstallmentsNamesTheNodeNotThePosition(t *testing.T) {
	one, two := 1, 2
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Seasons: []model.Season{
			{ID: "show-s1p1", Number: 1, Part: &one},
			{ID: "show-s1p2", Number: 1, Part: &two, ExternalIDs: model.ExternalIDs{AnilistID: 999}},
		},
	}
	report := &Report{}
	b.resolveInstallments(s, report)

	got := report.String()
	if !strings.Contains(got, "season show-s1p2") {
		t.Errorf("the node must be named:\n%s", got)
	}
	if strings.Contains(got, "season 2 ") {
		t.Errorf("a slot index must not be reported as a season number:\n%s", got)
	}
}

// Half of upstream's entries carry no AniList id — works listed on Anime News
// Network, MyAnimeList or anisearch and nowhere AniList reaches. They resolve
// from the series' own title like any other installment; what is different is
// that there is no id to write, so the entry itself comes back for the fills to
// read and the node keeps no anilistId at all.
const unlistedDB = `{"data":[
 {"sources":["https://animenewsnetwork.com/encyclopedia/anime.php?id=33926"],
  "title":"Unlisted Show","type":"TV","episodes":12,
  "animeSeason":{"season":"WINTER","year":2026},"synonyms":["ショー"]}
]}`

func TestResolveInstallmentsCarriesAWorkAniListDoesNotList(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, unlistedDB)})
	s := &model.Series{
		ID:      "show",
		Titles:  model.Title{Original: "ショー"},
		Seasons: []model.Season{{ID: "show-s1", Number: 1}},
	}
	report := &Report{}
	resolved := b.resolveInstallments(s, report)

	if got := s.Seasons[0].ExternalIDs.AnilistID; got != 0 {
		t.Errorf("anilistId = %d, want none: upstream has no AniList entry to name", got)
	}
	entry, ok := resolved[&s.Seasons[0].ExternalIDs]
	if !ok {
		t.Fatal("the entry itself must come back; it is the only handle on a work with no id")
	}
	if entry.Title != "Unlisted Show" {
		t.Errorf("resolved to %q", entry.Title)
	}
	// Counted apart from the id fractions: it contributes no id to them.
	if report.Coverage.Unlisted != 1 || report.Coverage.Derived != 0 {
		t.Errorf("coverage = %+v, want one unlisted installment and no derived id", report.Coverage)
	}
	if !report.Empty() {
		t.Errorf("a work AniList does not carry is not a finding: %v", report.Notes)
	}
}

// Upstream's entries are per-provider records merged where the providers
// cross-link, and that merge lags: the same work appears once with nine
// providers including AniList and again with Anime News Network alone. Counting
// both would make a one-season series look like a two-season one and stop it
// resolving at all, so the merged record is what a season pairs with.
const shadowedDB = `{"data":[
 {"sources":["https://animenewsnetwork.com/encyclopedia/anime.php?id=33926"],
  "title":"The Case Book of Show","type":"TV","episodes":0,
  "animeSeason":{"season":"UNDEFINED","year":2026},"synonyms":["ショー"]},
 {"sources":["https://anilist.co/anime/100","https://anidb.net/anime/7"],
  "title":"Show","type":"TV","episodes":12,
  "animeSeason":{"season":"WINTER","year":2026},"synonyms":["ショー"]}
]}`

func TestResolveInstallmentsPrefersTheAniListBearingRecord(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, shadowedDB)})
	s := &model.Series{
		ID:      "show",
		Titles:  model.Title{Original: "ショー"},
		Seasons: []model.Season{{ID: "show-s1", Number: 1}},
	}
	report := &Report{}
	resolved := b.resolveInstallments(s, report)

	if got := s.Seasons[0].ExternalIDs.AnilistID; got != 100 {
		t.Errorf("anilistId = %d, want 100 from the merged record", got)
	}
	if _, ok := resolved[&s.Seasons[0].ExternalIDs]; ok {
		t.Error("an installment with an id needs no entry carried beside it")
	}
	if report.Coverage.Unlisted != 0 {
		t.Errorf("the bare record is the same work, not an installment: %+v", report.Coverage)
	}
}

// The second half of upstream is consulted, not ignored: when the merged
// records cannot account for the installments — a season upstream has an
// AniList entry for and one it does not — the whole pool is tried, and each
// installment gets whichever kind of answer exists for it.
const mixedDB = `{"data":[
 {"sources":["https://anilist.co/anime/100"],"title":"Show","type":"TV","episodes":12,
  "animeSeason":{"season":"FALL","year":2020},"synonyms":["ショー"]},
 {"sources":["https://animenewsnetwork.com/encyclopedia/anime.php?id=1"],
  "title":"Show 2nd Season","type":"TV","episodes":13,
  "animeSeason":{"season":"SPRING","year":2026},"synonyms":["ショー"]}
]}`

func TestResolveInstallmentsFallsBackWhenTheMergedRecordsFallShort(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, mixedDB)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Seasons: []model.Season{
			{ID: "show-s1", Number: 1},
			{ID: "show-s2", Number: 2},
		},
	}
	report := &Report{}
	resolved := b.resolveInstallments(s, report)

	if got := s.Seasons[0].ExternalIDs.AnilistID; got != 100 {
		t.Errorf("season 1 anilistId = %d, want 100", got)
	}
	if got := s.Seasons[1].ExternalIDs.AnilistID; got != 0 {
		t.Errorf("season 2 anilistId = %d, want none", got)
	}
	entry, ok := resolved[&s.Seasons[1].ExternalIDs]
	if !ok || entry.Title != "Show 2nd Season" {
		t.Errorf("season 2 must carry its own entry, got %+v", entry)
	}
	if report.Coverage.Derived != 1 || report.Coverage.Unlisted != 1 {
		t.Errorf("coverage = %+v, want one derived id and one unlisted installment", report.Coverage)
	}
}

// An authored id against a record that has none is not a disagreement: the
// record makes no claim about which AniList entry this is, and what it usually
// is instead is upstream's second, un-merged record of the very work the id
// names. The id stands, and the build says nothing.
func TestResolveInstallmentsSaysNothingAboutAnUnlistedRecord(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, mixedDB)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Seasons: []model.Season{
			{ID: "show-s1", Number: 1},
			{ID: "show-s2", Number: 2, ExternalIDs: model.ExternalIDs{AnilistID: 200}},
		},
	}
	report := &Report{}
	resolved := b.resolveInstallments(s, report)

	if got := s.Seasons[1].ExternalIDs.AnilistID; got != 200 {
		t.Errorf("season 2 anilistId = %d, want the authored 200", got)
	}
	if _, ok := resolved[&s.Seasons[1].ExternalIDs]; ok {
		t.Error("the authored id decides; the record must not be carried beside it")
	}
	if !report.Empty() {
		t.Errorf("a record with no id contradicts nothing: %v", report.Notes)
	}
}

// The ordering ladder the pairing hangs off: airing year, then the quarter
// within it, then the AniList id, then upstream's own order. Each rung decides
// only what the one above left equal, and an entry with no airing window at all
// sorts last rather than first — an announced installment is the newest thing
// there is, not the oldest.
func TestAiredEarlierFallsThroughToUpstreamOrder(t *testing.T) {
	const ladder = `{"data":[
	 {"sources":["https://anilist.co/anime/300"],"title":"winter-300","type":"TV",
	  "animeSeason":{"season":"WINTER","year":2020},"synonyms":["名"]},
	 {"sources":["https://anilist.co/anime/200"],"title":"fall-200","type":"TV",
	  "animeSeason":{"season":"FALL","year":2020},"synonyms":["名"]},
	 {"sources":["https://anilist.co/anime/100"],"title":"winter-100","type":"TV",
	  "animeSeason":{"season":"WINTER","year":2020},"synonyms":["名"]},
	 {"sources":["https://anidb.net/anime/1"],"title":"winter-unlisted-first","type":"TV",
	  "animeSeason":{"season":"WINTER","year":2020},"synonyms":["名"]},
	 {"sources":["https://anidb.net/anime/2"],"title":"winter-unlisted-second","type":"TV",
	  "animeSeason":{"season":"WINTER","year":2020},"synonyms":["名"]},
	 {"sources":["https://anilist.co/anime/400"],"title":"undated","type":"TV","synonyms":["名"]},
	 {"sources":["https://anilist.co/anime/500"],"title":"earlier-year","type":"TV",
	  "animeSeason":{"season":"FALL","year":2019},"synonyms":["名"]}
	]}`
	entries := offlineFrom(t, ladder).Titled("名")
	sort.Slice(entries, func(i, j int) bool { return airedEarlier(entries[i], entries[j]) })

	want := []string{
		"earlier-year",           // the earlier year wins outright
		"winter-unlisted-first",  // same window: no id sorts as 0, upstream order breaks the tie
		"winter-unlisted-second", //
		"winter-100",             // then by id within the window
		"winter-300",             //
		"fall-200",               // a later quarter of the same year
		"undated",                // no window at all: newest, not oldest
	}
	for i, w := range want {
		if entries[i].Title != w {
			t.Fatalf("position %d = %q, want %q (full order %v)", i, entries[i].Title, w, titlesOf(entries))
		}
	}
}

// titlesOf names an ordering for a failure message.
func titlesOf(entries []offlinedb.Anime) []string {
	out := make([]string, len(entries))
	for i, a := range entries {
		out[i] = a.Title
	}
	return out
}

// Entries with no AniList id all answer 0, so the id can no longer break a tie
// between two that aired in the same quarter. Upstream's own order does, and
// has to: an unstable order here resolves the same override differently between
// two runs, which is churn in data/.
func TestResolveInstallmentsIsDeterministicAmongUnlistedRecords(t *testing.T) {
	const sameQuarter = `{"data":[
	 {"sources":["https://animenewsnetwork.com/encyclopedia/anime.php?id=1"],
	  "title":"First Listed","type":"TV","episodes":12,
	  "animeSeason":{"season":"WINTER","year":2026},"synonyms":["ショー"]},
	 {"sources":["https://animenewsnetwork.com/encyclopedia/anime.php?id=2"],
	  "title":"Second Listed","type":"TV","episodes":12,
	  "animeSeason":{"season":"WINTER","year":2026},"synonyms":["ショー"]}
	]}`
	b := New(Sources{Offline: offlineFrom(t, sameQuarter)})
	for run := range 20 {
		s := &model.Series{
			ID:     "show",
			Titles: model.Title{Original: "ショー"},
			Seasons: []model.Season{
				{ID: "show-s1", Number: 1},
				{ID: "show-s2", Number: 2},
			},
		}
		resolved := b.resolveInstallments(s, &Report{})
		if got := resolved[&s.Seasons[0].ExternalIDs].Title; got != "First Listed" {
			t.Fatalf("run %d: season 1 resolved to %q, want the entry upstream lists first", run, got)
		}
		if got := resolved[&s.Seasons[1].ExternalIDs].Title; got != "Second Listed" {
			t.Fatalf("run %d: season 2 resolved to %q", run, got)
		}
	}
}
