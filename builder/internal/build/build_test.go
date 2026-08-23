package build

import (
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/animelists"
	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/builder/internal/testsupport"
	"github.com/michael-freling/anime-metadata-db/internal/model"
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

func TestInferTitle(t *testing.T) {
	// Latin title + native synonym: native -> original/ja, Latin -> ja-Latn.
	// The Latin form is a romanization, not an English title: upstream says
	// "Kimetsu no Yaiba", and only a human can say "Demon Slayer".
	title := inferTitle(offlinedb.Anime{Title: "Kimetsu no Yaiba", Synonyms: []string{"x", "鬼滅の刃"}})
	if title.Original != "鬼滅の刃" || title.Translations["ja"] != "鬼滅の刃" || title.Translations["ja-Latn"] != "Kimetsu no Yaiba" {
		t.Errorf("unexpected title: %+v", title)
	}
	if _, ok := title.Translations["en"]; ok {
		t.Errorf("a romanization must not be filed as English: %+v", title.Translations)
	}

	// Native title, no latin: ja only, no en.
	title = inferTitle(offlinedb.Anime{Title: "鬼滅の刃"})
	if title.Original != "鬼滅の刃" || title.Translations["ja"] != "鬼滅の刃" {
		t.Errorf("native-only title: %+v", title)
	}
	if _, ok := title.Translations["ja-Latn"]; ok {
		t.Errorf("native-only should have no romanization: %+v", title.Translations)
	}

	// No native form anywhere: the Latin string is the title itself, not a
	// romanization of one — Fate/stay night is written that way in Japan too.
	title = inferTitle(offlinedb.Anime{Title: "Fate/stay night", Synonyms: []string{"FSN"}})
	if title.Original != "Fate/stay night" || len(title.Translations) != 0 {
		t.Errorf("no-native: %+v", title)
	}
}

// The script decides the language, because assuming Japanese mislabelled every
// Korean title: a series added without a hand-authored title would have had its
// Hangul filed under `ja`, and then served to Japanese-language requests while
// Korean ones got nothing.
func TestInferTitleReadsTheScript(t *testing.T) {
	for _, tc := range []struct {
		name, title, syn  string
		wantLang, wantRom string
	}{
		{"kana is Japanese", "Kimetsu no Yaiba", "鬼滅の刃", "ja", "ja-Latn"},
		{"hangul is Korean", "Metal Cardbot W", "메탈카드봇W", "ko", "ko-Latn"},
		// Han characters are shared, so this one is a guess — see below.
		{"han alone defaults to Japanese", "Jujutsu Kaisen", "呪術廻戦", "ja", "ja-Latn"},
	} {
		got := inferTitle(offlinedb.Anime{Title: tc.title, Synonyms: []string{tc.syn}})
		if got.Translations[tc.wantLang] != tc.syn {
			t.Errorf("%s: translations[%s] = %q, want %q", tc.name, tc.wantLang, got.Translations[tc.wantLang], tc.syn)
		}
		if got.Translations[tc.wantRom] != tc.title {
			t.Errorf("%s: translations[%s] = %q, want %q", tc.name, tc.wantRom, got.Translations[tc.wantRom], tc.title)
		}
		if len(got.Translations) != 2 {
			t.Errorf("%s: %+v, want exactly the language and its romanization", tc.name, got.Translations)
		}
	}
}

// A Han-only title could be Japanese or Chinese, so the build says which way it
// guessed rather than deciding quietly. Nothing else in the pipeline can tell
// the difference, so this report is the only thing standing between a wrong
// guess and a shipped record.
func TestFillTitlesReportsAnAssumedLanguage(t *testing.T) {
	report := &Report{}
	fillTitles("season x", &model.Title{}, offlinedb.Anime{Title: "Jujutsu Kaisen", Synonyms: []string{"呪術廻戦"}}, report)
	if !strings.Contains(report.String(), "Han characters") {
		t.Errorf("a Han-only original should be reported as an assumption, got %q", report.String())
	}

	// Two uncertain fills report a line each, and their order has to be the
	// same on every run: a report a reader cannot diff against the last one is
	// worth much less. Map iteration order made it vary.
	first := &Report{}
	fillTitles("season z", &model.Title{}, offlinedb.Anime{Title: "Jujutsu Kaisen", Synonyms: []string{"呪術廻戦"}}, first)
	for i := 0; i < 20; i++ {
		again := &Report{}
		fillTitles("season z", &model.Title{}, offlinedb.Anime{Title: "Jujutsu Kaisen", Synonyms: []string{"呪術廻戦"}}, again)
		if again.String() != first.String() {
			t.Fatalf("report order varies between runs:\n%s\nvs\n%s", first.String(), again.String())
		}
	}

	// A title with kana is not a guess and must not add noise to the report.
	quiet := &Report{}
	fillTitles("season y", &model.Title{}, offlinedb.Anime{Title: "Kimetsu no Yaiba", Synonyms: []string{"鬼滅の刃"}}, quiet)
	if strings.Contains(quiet.String(), "Han characters") {
		t.Errorf("kana is unambiguous and should not be reported: %q", quiet.String())
	}
}

func TestFillTitlesMerge(t *testing.T) {
	a := offlinedb.Anime{Title: "Kimetsu no Yaiba", Synonyms: []string{"鬼滅の刃"}}

	// An override that authored only translations.en keeps it, and gains ja +
	// original from the source.
	dst := &model.Title{Translations: map[string]string{"en": "Demon Slayer"}}
	fillTitles("season x", dst, a, &Report{})
	if dst.Translations["en"] != "Demon Slayer" {
		t.Errorf("authored en should be kept, got %q", dst.Translations["en"])
	}
	if dst.Translations["ja"] != "鬼滅の刃" {
		t.Errorf("ja should be filled, got %q", dst.Translations["ja"])
	}
	if dst.Original != "鬼滅の刃" {
		t.Errorf("original should be filled, got %q", dst.Original)
	}

	// An empty title gets the native form and its romanization.
	empty := &model.Title{}
	fillTitles("season y", empty, a, &Report{})
	if empty.Translations["ja-Latn"] != "Kimetsu no Yaiba" || empty.Translations["ja"] != "鬼滅の刃" {
		t.Errorf("empty title not filled with both: %+v", empty.Translations)
	}

	// An authored value wins for its own language, which is what keeps a
	// rebuild idempotent: the inferred title must never clobber a hand-written
	// one. This used to be covered by the `en` case above, until inferTitle
	// stopped emitting `en` at all — so it is pinned on the tags it does emit.
	authored := &model.Title{
		Original:     "authored 原題",
		Translations: map[string]string{"ja": "authored ja", "ja-Latn": "Authored Romaji"},
	}
	fillTitles("season w", authored, a, &Report{})
	if authored.Original != "authored 原題" {
		t.Errorf("authored original should be kept, got %q", authored.Original)
	}
	if authored.Translations["ja"] != "authored ja" || authored.Translations["ja-Latn"] != "Authored Romaji" {
		t.Errorf("authored translations should be kept, got %+v", authored.Translations)
	}

	// An authored romanization names the title's language, and the source's
	// text cannot outvote it. Asserting only that ja-Latn is absent used to
	// pass while a bare `ja` key — and a Japanese `original` — were quietly
	// filled in alongside the authored Korean one.
	korean := &model.Title{Translations: map[string]string{"ko-Latn": "Metal Cardbot W"}}
	report := &Report{}
	fillTitles("season z", korean, a, report)
	if korean.Translations["ko-Latn"] != "Metal Cardbot W" || len(korean.Translations) != 1 {
		t.Errorf("authored Korean title gained fields from a Japanese source: %+v", korean.Translations)
	}
	if korean.Original != "" {
		t.Errorf("original filled from a source in another language: %q", korean.Original)
	}
	if strings.Contains(report.String(), "Author the language explicitly") {
		t.Errorf("the language was authored; the report should not ask for it: %q", report.String())
	}

	// A romanization naming the *same* language as the source is not a
	// disagreement, so the rest of the title still fills.
	japanese := &model.Title{Translations: map[string]string{"ja-Latn": "Authored Romaji"}}
	fillTitles("season zz", japanese, a, &Report{})
	if japanese.Original != "鬼滅の刃" || japanese.Translations["ja"] != "鬼滅の刃" {
		t.Errorf("a matching romanization should not block the fill: %+v", japanese)
	}
	if japanese.Translations["ja-Latn"] != "Authored Romaji" {
		t.Errorf("authored romanization should be kept: %+v", japanese.Translations)
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
	// An unauthored season is not given a fabricated title.
	if !s.Seasons[0].Titles.IsZero() {
		t.Errorf("unauthored season should have no auto-filled title: %+v", s.Seasons[0].Titles)
	}
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

func TestBuildSeasonTitles(t *testing.T) {
	db := `{"data":[
	  {"sources":["https://anilist.co/anime/1"],"title":"Foo","type":"TV","episodes":12,"synonyms":["フー"]},
	  {"sources":["https://anilist.co/anime/2"],"title":"Foo 2nd Season","type":"TV","episodes":12,"synonyms":["フー2"]}
	]}`
	b := New(mustSources(t, db, "<anime-list/>", "<anime-movieset-list/>"))
	o := overrides.Override{Series: &model.Series{ID: "foo", Seasons: []model.Season{
		// No authored title -> stays title-less (no fabricated "Foo"/"フー").
		{ID: "foo-s1", Number: 1, ExternalIDs: model.ExternalIDs{AnilistID: 1}},
		// Authored arc name -> kept, with native name filled in.
		{ID: "foo-s2", Number: 1, Part: intp(2),
			Titles:      model.Title{Translations: map[string]string{"en": "Second Cour"}},
			ExternalIDs: model.ExternalIDs{AnilistID: 2}},
	}}}
	rec, _, err := b.Build(o)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Series.Seasons[0].Titles.IsZero() {
		t.Errorf("unauthored season should have no title: %+v", rec.Series.Seasons[0].Titles)
	}
	s2 := rec.Series.Seasons[1].Titles
	if s2.Translations["en"] != "Second Cour" || s2.Translations["ja"] == "" {
		t.Errorf("authored season should keep en and gain ja: %+v", s2)
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
