package build

import (
	"fmt"
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// offlineFrom builds a database from an offline-database-shaped document, so
// these tests exercise the same parse path a real build goes through.
func offlineFrom(t *testing.T, doc string) *offlinedb.Database {
	t.Helper()
	db, err := offlinedb.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// A sequel chain: 1 links to 2, 2 to 3. Nothing links 1 to 3 directly, which is
// how upstream models a multi-season show and why the check walks.
const chainDB = `{"data":[
 {"sources":["https://anilist.co/anime/1"],"title":"S1","type":"TV","episodes":12,
  "animeSeason":{"season":"FALL","year":2020},
  "relatedAnime":["https://anilist.co/anime/2"]},
 {"sources":["https://anilist.co/anime/2"],"title":"S2","type":"TV","episodes":12,
  "animeSeason":{"season":"SPRING","year":2022},
  "relatedAnime":["https://anilist.co/anime/1","https://anilist.co/anime/3"]},
 {"sources":["https://anilist.co/anime/3"],"title":"S3","type":"TV","episodes":12,
  "animeSeason":{"season":"WINTER","year":2024},
  "relatedAnime":["https://anilist.co/anime/2"]},
 {"sources":["https://anilist.co/anime/99"],"title":"Unrelated","type":"TV","episodes":12,
  "animeSeason":{"season":"FALL","year":1999}}
]}`

// season builds a numbered season carrying an AniList id and a release.
func season(id string, number, anilist, year int, quarter model.ReleaseSeason) model.Season {
	return model.Season{
		ID: id, Number: number,
		ReleaseYear: year, ReleaseSeason: quarter,
		ExternalIDs: model.ExternalIDs{AnilistID: anilist},
	}
}

// The catalogue's own shape: every installment linked, in order. Nothing to say.
func TestCheckAnilistIDsSilentOnAWellFormedSeries(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, chainDB)})
	s := &model.Series{ID: "chain", Seasons: []model.Season{
		season("chain-s1", 1, 1, 2020, model.SeasonFall),
		season("chain-s2", 2, 2, 2022, model.SeasonSpring),
		season("chain-s3", 3, 3, 2024, model.SeasonWinter),
	}}
	report := &Report{}
	b.checkAnilistIDs(s, report)

	if !report.Empty() {
		t.Errorf("a correct series must produce no notes, got %v", report.Notes)
	}
}

// The check that catches an id from the wrong show entirely. The offending
// season is the one named — an earlier version seeded one walk from the lowest
// id and, when that was the bad one, reported every other season instead.
func TestCheckAnilistIDsNamesTheUnlinkedInstallment(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, chainDB)})
	s := &model.Series{ID: "chain", Seasons: []model.Season{
		season("chain-s1", 1, 1, 2020, model.SeasonFall),
		season("chain-s2", 2, 2, 2022, model.SeasonSpring),
		season("chain-s3", 3, 99, 1999, model.SeasonFall), // wrong show
	}}
	report := &Report{}
	b.checkSameWork(s, report)

	got := report.String()
	if !strings.Contains(got, "anilistId 99 is not linked") {
		t.Errorf("the unlinked id must be named:\n%s", got)
	}
	for _, innocent := range []string{"anilistId 1 ", "anilistId 2 "} {
		if strings.Contains(got, innocent) {
			t.Errorf("a linked id must not be reported (%q):\n%s", innocent, got)
		}
	}
}

// With two installments and no link between them, neither is more credible than
// the other, so both are named rather than one being picked.
func TestCheckSameWorkNamesBothWhenAPairIsUnlinked(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, chainDB)})
	s := &model.Series{ID: "pair", Seasons: []model.Season{
		season("pair-s1", 1, 1, 2020, model.SeasonFall),
		season("pair-s2", 2, 99, 1999, model.SeasonFall),
	}}
	report := &Report{}
	b.checkSameWork(s, report)

	if len(report.Notes) != 2 {
		t.Errorf("expected both ids named, got %v", report.Notes)
	}
}

// The chronology check catches what the graph one cannot: an id naming the
// wrong installment of the *right* work, where both entries are siblings and
// every link is intact.
func TestCheckSeasonChronologyCatchesASwappedSibling(t *testing.T) {
	s := &model.Series{ID: "chain", Seasons: []model.Season{
		season("chain-s1", 1, 3, 2024, model.SeasonWinter), // S3's id on season 1
		season("chain-s2", 2, 1, 2020, model.SeasonFall),
	}}
	report := &Report{}
	checkSeasonChronology(s, report)

	got := report.String()
	// Both are named and neither is accused: season 1 is the one carrying the
	// wrong id here, so blaming season 2 — as an earlier version did — would
	// point a reader at the innocent one.
	if !strings.Contains(got, "season 2 (anilistId 1) aired FALL 2020, before season 1 (anilistId 3) aired WINTER 2024") {
		t.Errorf("expected both seasons named with their ids:\n%s", got)
	}
	if !strings.Contains(got, "one of the two names the wrong installment") {
		t.Errorf("the note must not accuse either one:\n%s", got)
	}
	// The graph check sees nothing wrong here — both ids are real siblings.
	b := New(Sources{Offline: offlineFrom(t, chainDB)})
	graph := &Report{}
	b.checkSameWork(s, graph)
	if !graph.Empty() {
		t.Errorf("the graph check should not fire on a swapped sibling: %v", graph.Notes)
	}
}

// Split cours share a season number and air in the same window, so comparing
// them against each other would report every split-cour series in the
// catalogue.
func TestCheckSeasonChronologyAllowsSplitCours(t *testing.T) {
	part := func(id string, number, part, anilist, year int, q model.ReleaseSeason) model.Season {
		s := season(id, number, anilist, year, q)
		s.Part = &part
		return s
	}
	s := &model.Series{ID: "cour", Seasons: []model.Season{
		part("cour-s2p1", 2, 1, 1, 2023, model.SeasonSummer),
		part("cour-s2p2", 2, 2, 2, 2023, model.SeasonSummer),
	}}
	report := &Report{}
	checkSeasonChronology(s, report)

	if !report.Empty() {
		t.Errorf("split cours of one season must not be compared: %v", report.Notes)
	}
}

// A season upstream has no airing window for says nothing about order, so it is
// skipped rather than treated as having aired in year zero — which would report
// every season that follows it.
func TestCheckSeasonChronologySkipsUndatedSeasons(t *testing.T) {
	s := &model.Series{ID: "undated", Seasons: []model.Season{
		season("undated-s1", 1, 1, 0, ""), // no release year upstream
		season("undated-s2", 2, 2, 2022, model.SeasonSpring),
	}}
	report := &Report{}
	checkSeasonChronology(s, report)

	if !report.Empty() {
		t.Errorf("an undated season must not produce a note: %v", report.Notes)
	}
}

// Movies and specials count as installments for the graph check: an id pasted
// onto one is as wrong as an id pasted onto a season.
func TestCheckSameWorkCoversMoviesAndSpecials(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, chainDB)})
	s := &model.Series{
		ID:       "chain",
		Seasons:  []model.Season{season("chain-s1", 1, 1, 2020, model.SeasonFall)},
		Movies:   []model.Movie{{ID: "chain-film", ExternalIDs: model.ExternalIDs{AnilistID: 2}}},
		Specials: []model.Special{{ID: "chain-ova", ExternalIDs: model.ExternalIDs{AnilistID: 99}}},
	}
	report := &Report{}
	b.checkSameWork(s, report)

	got := report.String()
	if !strings.Contains(got, "special chain-ova") || !strings.Contains(got, "anilistId 99") {
		t.Errorf("the special's wrong id must be named:\n%s", got)
	}
	if strings.Contains(got, "chain-film") {
		t.Errorf("the movie is linked and must not be reported:\n%s", got)
	}
}

// A single installment has nothing to be consistent with, and most of the
// catalogue is single-installment: a note each would bury the real findings.
func TestCheckSameWorkSilentOnALoneInstallment(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, chainDB)})
	s := &model.Series{ID: "solo", Seasons: []model.Season{
		season("solo-s1", 1, 99, 1999, model.SeasonFall),
	}}
	report := &Report{}
	b.checkSameWork(s, report)

	if !report.Empty() {
		t.Errorf("a lone installment cannot be checked: %v", report.Notes)
	}
}

// The build must run with no offline database — a fixture that supplies none —
// rather than panic on a nil index.
func TestCheckSameWorkWithoutOfflineDatabase(t *testing.T) {
	b := New(Sources{})
	s := &model.Series{ID: "x", Seasons: []model.Season{
		season("x-s1", 1, 1, 2020, model.SeasonFall),
		season("x-s2", 2, 99, 1999, model.SeasonFall),
	}}
	report := &Report{}
	b.checkSameWork(s, report)

	if !report.Empty() {
		t.Errorf("no source means no check: %v", report.Notes)
	}
}

// The counts are what tell a reader whether an empty report means "clean" or
// "nothing was looked at", so they have to agree with the notes beside them: an
// id that was checked and failed is neither corroborated nor unverifiable.
func TestCheckSameWorkCoverageCounts(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, chainDB)})

	linked := &Report{}
	b.checkSameWork(&model.Series{ID: "chain", Seasons: []model.Season{
		season("chain-s1", 1, 1, 2020, model.SeasonFall),
		season("chain-s2", 2, 2, 2022, model.SeasonSpring),
	}}, linked)
	if got := linked.Coverage; got.Corroborated != 2 || got.Alone != 0 || got.Considered != 2 {
		t.Errorf("both linked: %+v", got)
	}

	alone := &Report{}
	b.checkSameWork(&model.Series{ID: "solo", Seasons: []model.Season{
		season("solo-s1", 1, 99, 1999, model.SeasonFall),
	}}, alone)
	if got := alone.Coverage; got.Alone != 1 || got.Corroborated != 0 || got.Considered != 1 {
		t.Errorf("lone installment: %+v", got)
	}

	// One good, one from another show: the good one counts, the bad one is
	// reported rather than counted, so the summary cannot claim it passed.
	mixed := &Report{}
	b.checkSameWork(&model.Series{ID: "chain", Seasons: []model.Season{
		season("chain-s1", 1, 1, 2020, model.SeasonFall),
		season("chain-s2", 2, 2, 2022, model.SeasonSpring),
		season("chain-s3", 3, 99, 1999, model.SeasonFall),
	}}, mixed)
	if got := mixed.Coverage; got.Corroborated != 2 || got.Alone != 0 {
		t.Errorf("a failing id must not be counted as checked: %+v", got)
	}
	// The denominator counts every id the checks were given, including the one
	// that failed. Summing the outcomes instead would shrink it exactly when
	// something was found, which is when the fraction matters most.
	if got := mixed.Coverage.Total(); got != 3 {
		t.Errorf("Total = %d, want every id considered", got)
	}
	if len(mixed.Notes) != 1 {
		t.Errorf("the failing id must still be reported: %v", mixed.Notes)
	}
}

// Reports are merged per record before the build sums them, so the counts have
// to survive a merge or the total silently under-reports.
func TestReportMergeSumsCoverage(t *testing.T) {
	a := &Report{Coverage: Coverage{Considered: 3, Corroborated: 2, Alone: 1}}
	a.Merge(&Report{Coverage: Coverage{Considered: 7, Corroborated: 3, Alone: 4}})
	if a.Coverage.Corroborated != 5 || a.Coverage.Alone != 5 {
		t.Errorf("merge lost counts: %+v", a.Coverage)
	}
	if a.Coverage.Total() != 10 {
		t.Errorf("Total = %d, want 10", a.Coverage.Total())
	}
	// A nil merge is a no-op elsewhere in Report and must stay one here.
	a.Merge(nil)
	if a.Coverage.Total() != 10 {
		t.Errorf("nil merge changed the counts: %+v", a.Coverage)
	}
}

// Coverage must not make a findings-free report look non-empty, or every clean
// build would print a report block.
func TestCoverageDoesNotMakeAReportNonEmpty(t *testing.T) {
	r := &Report{Coverage: Coverage{Considered: 9, Corroborated: 9}}
	if !r.Empty() {
		t.Error("counts alone are not findings")
	}
}

// Two installments given the same id is the one mistake the other checks cannot
// see — a duplicate is trivially linked to and in order with itself. It used to
// vanish into a map key, leaving the series looking like a single installment.
func TestCheckSameWorkReportsADuplicateID(t *testing.T) {
	b := New(Sources{Offline: offlineFrom(t, chainDB)})
	s := &model.Series{ID: "chain", Seasons: []model.Season{
		season("chain-s1", 1, 1, 2020, model.SeasonFall),
		season("chain-s2", 2, 1, 2022, model.SeasonSpring), // same id, pasted twice
	}}
	report := &Report{}
	b.checkSameWork(s, report)

	got := report.String()
	if !strings.Contains(got, "anilistId 1 is authored on more than one installment") {
		t.Errorf("the duplicate must be reported:\n%s", got)
	}
	for _, name := range []string{"season chain-s1", "season chain-s2"} {
		if !strings.Contains(got, name) {
			t.Errorf("every node carrying it must be named (%s):\n%s", name, got)
		}
	}
	// The duplicate is a finding, so it must not also be filed as "nothing to
	// check against". Counting it under Alone put a series with a reported
	// defect into the unverifiable bucket, and the provenance line then read as
	// if the checks had had nothing to say about it.
	if c := report.Coverage; c.Alone != 0 || c.Corroborated != 0 {
		t.Errorf("a duplicate was checked and failed, so it is neither alone nor corroborated: %+v", c)
	}
	// It gates: two installments are two different works whatever upstream
	// says, so this one is safe to fail CI on.
	if g := report.Gating(); len(g) != 1 || g[0].Code != CodeDuplicate {
		t.Errorf("the duplicate must be a gating finding, got %+v", g)
	}
	// Both are still counted, or the coverage line would say one id where the
	// override carries two.
	if got := report.Coverage.Considered; got != 2 {
		t.Errorf("Authored = %d, want 2", got)
	}
}

// A walk that stops at the cap has not shown the id is unlinked, only that
// nobody looked far enough. Saying so is the difference between a finding and
// an accusation.
func TestCheckSameWorkSaysWhenTheWalkWasTruncated(t *testing.T) {
	// A chain longer than the cap, so the walk runs out of budget before it
	// reaches the far end.
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i := 1; i <= relatedWalkCap+5; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"sources":["https://anilist.co/anime/%d"],"title":"n%d","type":"TV","episodes":1,
		  "animeSeason":{"season":"FALL","year":2000},"relatedAnime":["https://anilist.co/anime/%d"]}`, i, i, i+1)
	}
	b.WriteString(`]}`)

	bl := New(Sources{Offline: offlineFrom(t, b.String())})
	s := &model.Series{ID: "long", Seasons: []model.Season{
		season("long-s1", 1, 1, 2000, model.SeasonFall),
		season("long-s2", 2, relatedWalkCap+4, 2001, model.SeasonFall),
	}}
	report := &Report{}
	bl.checkSameWork(s, report)

	if !strings.Contains(report.String(), "within the walk limit, so this is inconclusive rather than wrong") {
		t.Errorf("a truncated walk must say so:\n%s", report.String())
	}
}

// Upstream leaves animeSeason.season blank while carrying the year. Reading
// that as "earliest" reported a season for airing before the one ahead of it
// when all that was known is that both aired the same year.
func TestCheckSeasonChronologyIgnoresAnUnknownQuarter(t *testing.T) {
	s := &model.Series{ID: "x", Seasons: []model.Season{
		season("x-s1", 1, 1, 2023, model.SeasonFall),
		season("x-s2", 2, 2, 2023, ""), // same year, quarter unknown upstream
	}}
	report := &Report{}
	checkSeasonChronology(s, report)

	if !report.Empty() {
		t.Errorf("an unknown quarter within the same year says nothing: %v", report.Notes)
	}
}

// A different year is still decisive, unknown quarter or not.
func TestCheckSeasonChronologyStillCatchesAWrongYear(t *testing.T) {
	s := &model.Series{ID: "x", Seasons: []model.Season{
		season("x-s1", 1, 1, 2023, model.SeasonFall),
		season("x-s2", 2, 2, 2019, ""),
	}}
	report := &Report{}
	checkSeasonChronology(s, report)

	if report.Empty() {
		t.Error("a season airing four years early is out of order whatever the quarter")
	}
}
