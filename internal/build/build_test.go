package build

import (
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/internal/model"
	"github.com/michael-freling/anime-metadata-db/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/internal/sources/animelists"
	"github.com/michael-freling/anime-metadata-db/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/testsupport"
)

func mustSources(t *testing.T, dbJSON, alXML, msXML string) Sources {
	t.Helper()
	off, err := offlinedb.Parse(strings.NewReader(dbJSON))
	if err != nil {
		t.Fatal(err)
	}
	al, err := animelists.ParseAnimeList(strings.NewReader(alXML))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := animelists.ParseMovieSetList(strings.NewReader(msXML))
	if err != nil {
		t.Fatal(err)
	}
	return Sources{Offline: off, AnimeList: al, MovieSets: ms}
}

func intp(n int) *int { return &n }

func TestHasNativeScript(t *testing.T) {
	if !hasNativeScript("鬼滅の刃") {
		t.Error("kanji should be native")
	}
	if !hasNativeScript("セイバー") {
		t.Error("katakana should be native")
	}
	if hasNativeScript("Fate/Zero 2011") {
		t.Error("latin should not be native")
	}
}

func TestInferTitle(t *testing.T) {
	// Latin title + native synonym.
	title, notes := inferTitle(offlinedb.Anime{Title: "Kimetsu no Yaiba", Synonyms: []string{"x", "鬼滅の刃"}})
	if title.Original != "鬼滅の刃" || title.Translations["en"] != "Kimetsu no Yaiba" {
		t.Errorf("unexpected title: %+v", title)
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 notes, got %v", notes)
	}

	// Native title, no latin.
	title, notes = inferTitle(offlinedb.Anime{Title: "鬼滅の刃"})
	if title.Original != "鬼滅の刃" || title.Translations != nil || len(notes) != 0 {
		t.Errorf("native-only: %+v notes=%v", title, notes)
	}

	// No native anywhere.
	title, notes = inferTitle(offlinedb.Anime{Title: "Fate/stay night", Synonyms: []string{"FSN"}})
	if title.Original != "" || title.Translations["en"] != "Fate/stay night" {
		t.Errorf("no-native: %+v", title)
	}
	if len(notes) != 2 { // "no native-script" + "assumed en"
		t.Errorf("expected 2 notes, got %v", notes)
	}
}

func TestPartOf(t *testing.T) {
	if partOf(nil) != 0 {
		t.Error("nil part should be 0")
	}
	if partOf(intp(2)) != 2 {
		t.Error("part pointer should deref")
	}
}

func TestOrderKey(t *testing.T) {
	d := model.NewDate(2019, 6, 15)
	if got := orderKey(&d, 0, ""); !got.Equal(d.Time) {
		t.Errorf("explicit date not used: %v", got)
	}
	got := orderKey(nil, 2019, model.SeasonFall)
	if got.Year() != 2019 || got.Month() != 10 {
		t.Errorf("season key wrong: %v", got)
	}
	noSeason := orderKey(nil, 2019, "")
	if noSeason.Month() != 1 {
		t.Errorf("missing season should default to January: %v", noSeason)
	}
}

func TestFillReleaseSeason(t *testing.T) {
	// From animeSeason.
	year, season := 0, model.ReleaseSeason("")
	fillReleaseSeason(&year, &season, nil, offlinedb.Anime{AnimeSeason: offlinedb.AnimeSeason{Season: "SPRING", Year: 2019}})
	if year != 2019 || season != model.SeasonSpring {
		t.Errorf("from animeSeason: %d %q", year, season)
	}

	// Derived from releaseDate when upstream is empty/invalid.
	d := model.NewDate(2020, 10, 16)
	year, season = 0, model.ReleaseSeason("")
	fillReleaseSeason(&year, &season, &d, offlinedb.Anime{AnimeSeason: offlinedb.AnimeSeason{Season: "BOGUS"}})
	if year != 2020 || season != model.SeasonFall {
		t.Errorf("derived from date: %d %q", year, season)
	}

	// Nothing to fill from: stays empty without panicking.
	year, season = 0, model.ReleaseSeason("")
	fillReleaseSeason(&year, &season, nil, offlinedb.Anime{})
	if year != 0 || season != "" {
		t.Errorf("expected empty, got %d %q", year, season)
	}
}

const fillSpecialDB = `{"data":[
  {"sources":["https://anilist.co/anime/1"],"title":"ona","type":"ONA","episodes":2},
  {"sources":["https://anilist.co/anime/2"],"title":"special","type":"SPECIAL","episodes":1},
  {"sources":["https://anilist.co/anime/3"],"title":"ova","type":"OVA","episodes":1},
  {"sources":["https://anilist.co/anime/4"],"title":"tv","type":"TV","episodes":1}
]}`

func TestFillSpecialFormat(t *testing.T) {
	b := New(mustSources(t, fillSpecialDB, "<anime-list/>", "<anime-movieset-list/>"))
	cases := []struct {
		anilist int
		want    model.SpecialFormat
	}{
		{1, model.FormatONA},
		{2, model.FormatSpecial},
		{3, model.FormatOVA},
		{4, model.FormatOVA}, // unknown upstream type defaults to OVA
	}
	for _, c := range cases {
		sp := &model.Special{ID: "sp", ExternalIDs: model.ExternalIDs{AnilistID: c.anilist}}
		if err := b.fillSpecial(sp, &Report{}); err != nil {
			t.Fatal(err)
		}
		if sp.Format != c.want {
			t.Errorf("anilist %d: format = %q want %q", c.anilist, sp.Format, c.want)
		}
	}

	// Authored format is preserved; episodes are generated from the count.
	sp := &model.Special{ID: "sp", Format: model.FormatONA, ExternalIDs: model.ExternalIDs{AnilistID: 1}}
	if err := b.fillSpecial(sp, &Report{}); err != nil {
		t.Fatal(err)
	}
	if sp.Format != model.FormatONA || len(sp.Episodes) != 2 {
		t.Errorf("authored format/episodes: %q %d", sp.Format, len(sp.Episodes))
	}
}

func TestFillMovieNoAnidb(t *testing.T) {
	db := `{"data":[{"sources":["https://anilist.co/anime/9"],"title":"Solo Movie","type":"MOVIE","episodes":1}]}`
	b := New(mustSources(t, db, "<anime-list/>", "<anime-movieset-list/>"))
	m := &model.Movie{ID: "m", ExternalIDs: model.ExternalIDs{AnilistID: 9}}
	report := &Report{}
	if err := b.fillMovie(m, report); err != nil {
		t.Fatal(err)
	}
	for _, n := range report.Notes {
		if strings.Contains(n.Message, "movie set") {
			t.Errorf("movie with no anidb should not get a set note: %v", report.Notes)
		}
	}
}

func TestBuildDemonSlayer(t *testing.T) {
	srcs := mustSources(t, testsupport.OfflineDBJSON, testsupport.AnimeListXML, testsupport.MovieSetXML)
	o, err := overrides.Parse([]byte(testsupport.DemonSlayerOverride), "series/demon-slayer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rec, report, err := New(srcs).Build(o)
	if err != nil {
		t.Fatal(err)
	}
	s := rec.Series
	if got := len(s.Seasons[0].Episodes); got != 26 {
		t.Fatalf("s1 episodes = %d", got)
	}
	if n := s.Seasons[0].Episodes[0].AbsoluteNumber; n == nil || *n != 1 {
		t.Errorf("first episode absolute = %v", n)
	}
	if n := s.Seasons[1].Episodes[6].AbsoluteNumber; n == nil || *n != 33 {
		t.Errorf("s2p1 last episode absolute = %v", n)
	}
	// Cross-filled TVDB id from anime-list.xml.
	if s.Seasons[0].ExternalIDs.TvdbID != 361069 {
		t.Errorf("tvdb id not cross-filled: %+v", s.Seasons[0].ExternalIDs)
	}
	// Alternate-cut film has no number; original Infinity film numbered after seasons.
	var mugen, infinity *model.Movie
	for i := range s.Movies {
		switch s.Movies[i].ID {
		case "ds-mugen-film":
			mugen = &s.Movies[i]
		case "ds-infinity":
			infinity = &s.Movies[i]
		}
	}
	if mugen.AbsoluteNumber != nil {
		t.Errorf("alternate cut should have no number, got %v", *mugen.AbsoluteNumber)
	}
	if infinity.AbsoluteNumber == nil || *infinity.AbsoluteNumber != 34 {
		t.Errorf("infinity absolute = %v, want 34", infinity.AbsoluteNumber)
	}
	if report.Empty() {
		t.Error("expected low-confidence title notes")
	}
	// A movie-set note should be present for the original film.
	var hasSetNote bool
	for _, n := range report.Notes {
		if strings.Contains(n.Message, "movie set") {
			hasSetNote = true
		}
	}
	if !hasSetNote {
		t.Error("expected a movie-set note")
	}
}

const fateDB = `{"data":[
  {"sources":["https://anilist.co/anime/356"],"title":"Fate/stay night","type":"TV","episodes":24,"animeSeason":{"season":"WINTER","year":2006}},
  {"sources":["https://anilist.co/anime/20724"],"title":"Heaven's Feel I","type":"MOVIE","episodes":1}
]}`

func TestBuildFranchiseNonNumbered(t *testing.T) {
	srcs := mustSources(t, fateDB, "<anime-list/>", "<anime-movieset-list/>")
	date := model.NewDate(2017, 10, 14)
	o := overrides.Override{
		Path: "franchises/fate.yaml",
		Franchise: &model.Franchise{
			ID:     "fate",
			Titles: model.Title{Translations: map[string]string{"en": "Fate"}},
			Series: []model.Series{{
				ID: "fate-stay-night",
				Seasons: []model.Season{{
					ID: "fsn-2006", Number: 1,
					Titles:      model.Title{Translations: map[string]string{"en": "Fate/stay night (2006)"}},
					ExternalIDs: model.ExternalIDs{AnilistID: 356},
				}},
				Movies: []model.Movie{{
					ID: "fsn-hf-1", ReleaseDate: &date,
					ExternalIDs: model.ExternalIDs{AnilistID: 20724},
				}},
			}},
			WatchOrders: []model.WatchOrder{{
				Name:    "Chronological",
				Entries: []model.WatchOrderEntry{{Ref: "fate-stay-night"}},
			}},
		},
		// not numbered
	}
	rec, _, err := New(srcs).Build(o)
	if err != nil {
		t.Fatal(err)
	}
	s := rec.Franchise.Series[0]
	// Non-numbered: no absolute numbers anywhere.
	for _, e := range s.Seasons[0].Episodes {
		if e.AbsoluteNumber != nil {
			t.Fatal("non-numbered series should have no absoluteNumber")
		}
	}
	// Authored season title preserved (override wins).
	if s.Seasons[0].Titles.Translations["en"] != "Fate/stay night (2006)" {
		t.Errorf("authored title overwritten: %+v", s.Seasons[0].Titles)
	}
	// Movie release year filled from its release date.
	if s.Movies[0].ReleaseYear != 2017 {
		t.Errorf("movie release year = %d", s.Movies[0].ReleaseYear)
	}
}

func TestBuildErrors(t *testing.T) {
	srcs := mustSources(t, fateDB, "<anime-list/>", "<anime-movieset-list/>")
	b := New(srcs)

	t.Run("unknown id", func(t *testing.T) {
		o := overrides.Override{Series: &model.Series{ID: "s", Seasons: []model.Season{{
			ID: "x", Number: 1, ExternalIDs: model.ExternalIDs{AnilistID: 999999},
		}}}}
		if _, _, err := b.Build(o); err == nil {
			t.Error("expected unknown id error")
		}
	})

	t.Run("missing anilistId", func(t *testing.T) {
		o := overrides.Override{Series: &model.Series{ID: "s", Seasons: []model.Season{{
			ID: "x", Number: 1,
		}}}}
		if _, _, err := b.Build(o); err == nil {
			t.Error("expected missing anilistId error")
		}
	})

	t.Run("empty record", func(t *testing.T) {
		if _, _, err := b.Build(overrides.Override{Path: "empty.yaml"}); err == nil {
			t.Error("expected empty record error")
		}
	})

	t.Run("movie unknown id", func(t *testing.T) {
		o := overrides.Override{Series: &model.Series{ID: "s", Movies: []model.Movie{{
			ID: "m", ExternalIDs: model.ExternalIDs{AnilistID: 7},
		}}}}
		if _, _, err := b.Build(o); err == nil {
			t.Error("expected movie unknown id error")
		}
	})

	t.Run("special unknown id", func(t *testing.T) {
		o := overrides.Override{Series: &model.Series{ID: "s", Specials: []model.Special{{
			ID: "sp", ExternalIDs: model.ExternalIDs{AnilistID: 7},
		}}}}
		if _, _, err := b.Build(o); err == nil {
			t.Error("expected special unknown id error")
		}
	})

	t.Run("validation failure propagates", func(t *testing.T) {
		// Two seasons share an id; both resolve, then validate rejects.
		o := overrides.Override{Series: &model.Series{ID: "s", Seasons: []model.Season{
			{ID: "dup", Number: 1, ExternalIDs: model.ExternalIDs{AnilistID: 356}},
			{ID: "dup", Number: 2, ExternalIDs: model.ExternalIDs{AnilistID: 356}},
		}}}
		if _, _, err := b.Build(o); err == nil {
			t.Error("expected validation error")
		}
	})

	t.Run("franchise second series error", func(t *testing.T) {
		o := overrides.Override{Franchise: &model.Franchise{ID: "f", Series: []model.Series{
			{ID: "ok", Seasons: []model.Season{{ID: "a", Number: 1, ExternalIDs: model.ExternalIDs{AnilistID: 356}}}},
			{ID: "bad", Seasons: []model.Season{{ID: "b", Number: 1, ExternalIDs: model.ExternalIDs{AnilistID: 123456}}}},
		}}}
		if _, _, err := b.Build(o); err == nil {
			t.Error("expected error from second series")
		}
	})
}
