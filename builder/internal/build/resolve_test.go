package build

import (
	"strings"
	"testing"

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
func TestResolveAnilistIDsAssignsEveryKind(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := showSeries()
	report := &Report{}
	b.resolveAnilistIDs(s, report)

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
func TestResolveAnilistIDsOrdersByAiringDate(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Seasons: []model.Season{
			{ID: "show-s2", Number: 2},
			{ID: "show-s1", Number: 1},
		},
	}
	b.resolveAnilistIDs(s, &Report{})

	if s.Seasons[0].ExternalIDs.AnilistID != 200 || s.Seasons[1].ExternalIDs.AnilistID != 100 {
		t.Errorf("ids follow the season number, not the file order: %+v", s.Seasons)
	}
}

// An authored id is never overwritten: it is the anchor the build reads every
// other fact through, and a resolution that disagrees is reported rather than
// applied. Which of the two is wrong is not something the build can know.
func TestResolveAnilistIDsNeverOverwritesAnAuthoredID(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := showSeries()
	s.Seasons[0].ExternalIDs.AnilistID = 999 // authored, and deliberately wrong
	report := &Report{}
	b.resolveAnilistIDs(s, report)

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
func TestResolveAnilistIDsCountsAgreement(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := showSeries()
	s.Seasons[0].ExternalIDs.AnilistID = 100
	s.Seasons[1].ExternalIDs.AnilistID = 200
	report := &Report{}
	b.resolveAnilistIDs(s, report)

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
func TestResolveAnilistIDsRefusesWhenCountsDisagree(t *testing.T) {
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
	b.resolveAnilistIDs(s, report)

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
func TestResolveAnilistIDsUnknownTitle(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := &model.Series{
		ID:      "nope",
		Titles:  model.Title{Original: "知らない題"},
		Seasons: []model.Season{{ID: "nope-s1", Number: 1}},
	}
	report := &Report{}
	b.resolveAnilistIDs(s, report)

	if s.Seasons[0].ExternalIDs.AnilistID != 0 {
		t.Error("an unknown title must resolve nothing")
	}
	if !report.Empty() {
		t.Errorf("nothing was found, so there is nothing to report here: %v", report.Notes)
	}
}

// The romanization is a second key, because upstream indexes some series under
// a Latin-script name and others under the native one.
func TestResolveAnilistIDsMatchesOnTheRomanization(t *testing.T) {
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
	b.resolveAnilistIDs(s, &Report{})

	if s.Seasons[0].ExternalIDs.AnilistID != 100 {
		t.Errorf("the romanization must be tried too, got %d", s.Seasons[0].ExternalIDs.AnilistID)
	}
}

// The build must run with no offline database rather than panic on a nil index.
func TestResolveAnilistIDsWithoutOfflineDatabase(t *testing.T) {
	b := New(Sources{})
	s := showSeries()
	b.resolveAnilistIDs(s, &Report{})

	if s.Seasons[0].ExternalIDs.AnilistID != 0 {
		t.Error("no source means no resolution")
	}
}

// Split cours share a season number, so the tie is broken on the id to keep the
// order fixed — two runs over one override must agree, or data/ churns.
func TestResolveAnilistIDsIsDeterministicForSplitCours(t *testing.T) {
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
		b.resolveAnilistIDs(s, &Report{})
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
func TestResolveAnilistIDsSkipsUnorderedKindsWithMoreThanOne(t *testing.T) {
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
	b.resolveAnilistIDs(s, report)

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
func TestResolveAnilistIDsStillHandlesASingleMovie(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := &model.Series{
		ID:     "show",
		Titles: model.Title{Original: "ショー"},
		Movies: []model.Movie{{ID: "show-film"}},
	}
	b.resolveAnilistIDs(s, &Report{})

	if s.Movies[0].ExternalIDs.AnilistID != 300 {
		t.Errorf("movie = %d, want 300", s.Movies[0].ExternalIDs.AnilistID)
	}
}

// A split cour sits in a later slot than its own number, so a disagreement is
// named by the node rather than by its position in the ordering.
func TestResolveAnilistIDsNamesTheNodeNotThePosition(t *testing.T) {
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
	b.resolveAnilistIDs(s, report)

	got := report.String()
	if !strings.Contains(got, "season show-s1p2") {
		t.Errorf("the node must be named:\n%s", got)
	}
	if strings.Contains(got, "season 2 ") {
		t.Errorf("a slot index must not be reported as a season number:\n%s", got)
	}
}
