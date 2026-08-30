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

// An authored id is the correction path for what the resolution gets wrong, so
// it has to survive — and its whole kind is left alone, since filling around it
// would mean guessing which candidates it already accounts for.
func TestResolveAnilistIDsLeavesAnAuthoredKindAlone(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, familyDB)})
	s := showSeries()
	s.Seasons[0].ExternalIDs.AnilistID = 999 // authored, and deliberately odd
	b.resolveAnilistIDs(s, &Report{})

	if s.Seasons[0].ExternalIDs.AnilistID != 999 {
		t.Error("an authored id was overwritten")
	}
	if s.Seasons[1].ExternalIDs.AnilistID != 0 {
		t.Errorf("the rest of an authored kind must be left alone, got %d", s.Seasons[1].ExternalIDs.AnilistID)
	}
	// Other kinds are independent, and still resolve.
	if s.Movies[0].ExternalIDs.AnilistID != 300 {
		t.Errorf("movie = %d, want 300", s.Movies[0].ExternalIDs.AnilistID)
	}
}

// The safety property the measurements turned on: when the counts disagree,
// nothing is assigned. A spare candidate would have to be attributed to some
// installment, and getting that wrong fills a season with another's episode
// count and says nothing.
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
	got := report.String()
	if !strings.Contains(got, "3 season(s) authored but 2 upstream") {
		t.Errorf("the refusal must say what did not line up:\n%s", got)
	}
	if report.Coverage.Derived != 0 {
		t.Errorf("nothing was derived, so nothing should be counted: %d", report.Coverage.Derived)
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
