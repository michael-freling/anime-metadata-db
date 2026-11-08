package importer

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/michael-freling/anime-metadata-db/src/builder/internal/sources/offlinedb"
)

// entry builds one upstream record. Sources and relations are given as whole
// URLs because that is what upstream publishes and what the resolution under
// test has to cope with.
type entry struct {
	title    string
	typ      string
	episodes int
	year     int
	season   string
	status   string
	sources  []string
	related  []string
	synonyms []string
}

func load(t *testing.T, entries ...entry) *offlinedb.Database {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i, e := range entries {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"title":` + q(e.title) +
			`,"type":"` + e.typ + `","episodes":` + itoa(e.episodes) +
			`,"status":` + q(e.status) +
			`,"animeSeason":{"season":"` + e.season + `","year":` + itoa(e.year) + `}` +
			`,"sources":[` + list(e.sources) + `]` +
			`,"relatedAnime":[` + list(e.related) + `]` +
			`,"synonyms":[` + list(e.synonyms) + `]}`)
	}
	b.WriteString(`]}`)
	db, err := offlinedb.Parse(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return db
}

func q(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func list(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = q(s)
	}
	return strings.Join(out, ",")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	if neg {
		return "-" + string(d)
	}
	return string(d)
}

func al(id string) string  { return "https://anilist.co/anime/" + id }
func mal(id string) string { return "https://myanimelist.net/anime/" + id }

func TestBuildTakesOnlyTVAndFilmsThatAired(t *testing.T) {
	db := load(t,
		entry{title: "A TV Show", typ: "TV", episodes: 12, year: 2020, sources: []string{al("1")}},
		entry{title: "A Film", typ: "MOVIE", episodes: 1, year: 2020, sources: []string{al("2")}},
		entry{title: "An OVA", typ: "OVA", episodes: 2, year: 2020, sources: []string{al("3")}},
		entry{title: "A Promo Clip", typ: "ONA", episodes: 1, year: 2020, sources: []string{al("4")}},
		entry{title: "A Special", typ: "SPECIAL", episodes: 1, year: 2020, sources: []string{al("5")}},
		// Announced but not aired: upstream carries it with no episodes.
		entry{title: "Not Out Yet", typ: "TV", episodes: 0, year: 2020, sources: []string{al("6")}},
		// Another year entirely.
		entry{title: "Last Year", typ: "TV", episodes: 12, year: 2019, sources: []string{al("7")}},
	)
	plan := Build(db, 2020, map[int]bool{999: true})

	var got []string
	for _, s := range plan.New {
		got = append(got, s.ID)
	}
	want := []string{"a-film", "a-tv-show"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("new series = %v, want %v", got, want)
	}
	// The out-of-scope count is reported rather than silently dropped, so a
	// year whose entries are mostly promos is visible as that.
	if plan.Skipped != 4 {
		t.Errorf("Skipped = %d, want 4 (OVA, ONA, SPECIAL, zero-episode)", plan.Skipped)
	}
}

func TestBuildSkipsWorksTheCatalogueAlreadyHas(t *testing.T) {
	db := load(t,
		entry{title: "Already Here", typ: "TV", episodes: 12, year: 2020, sources: []string{al("100")}},
		entry{title: "Brand New", typ: "TV", episodes: 12, year: 2020, sources: []string{al("101")}},
	)
	plan := Build(db, 2020, map[int]bool{100: true})

	if len(plan.Present) != 1 || plan.Present[0] != "Already Here" {
		t.Errorf("Present = %v, want [Already Here]", plan.Present)
	}
	if len(plan.New) != 1 || plan.New[0].ID != "brand-new" {
		t.Errorf("New = %+v, want just brand-new", plan.New)
	}
}

// TestBuildReportsAMergeLinkedByANonAnilistURL is the regression that matters
// most. Upstream records a relation as whichever provider's URL it has, and
// reading only the AniList ones saw 8 of the 29 real cases in 2020 — so a new
// installment of an existing series would have been written as a second,
// duplicate series, which is the one import mistake nothing downstream can
// detect.
func TestBuildReportsAMergeLinkedByANonAnilistURL(t *testing.T) {
	db := load(t,
		// In the catalogue already, and reachable only by its MAL URL.
		entry{title: "Existing Season 1", typ: "TV", episodes: 12, year: 2018,
			sources: []string{al("500"), mal("900")}},
		entry{title: "Existing Season 2", typ: "TV", episodes: 12, year: 2020,
			sources: []string{al("501")}, related: []string{mal("900")}},
	)
	plan := Build(db, 2020, map[int]bool{500: true})

	if len(plan.New) != 0 {
		t.Errorf("wrote %d new series for an installment of an existing one: %+v", len(plan.New), plan.New)
	}
	if len(plan.Merges) != 1 {
		t.Fatalf("Merges = %+v, want one", plan.Merges)
	}
	if got := plan.Merges[0]; got.Title != "Existing Season 2" || len(got.Into) != 1 || got.Into[0] != 500 {
		t.Errorf("Merges[0] = %+v, want Existing Season 2 -> [500]", got)
	}
}

func TestBuildGroupsInstallmentsSharingTheSeriesTitle(t *testing.T) {
	db := load(t,
		entry{title: "Show Second Cour", typ: "TV", episodes: 12, year: 2020, season: "FALL",
			sources: []string{al("11")}, synonyms: []string{"Show"}},
		entry{title: "Show", typ: "TV", episodes: 12, year: 2020, season: "SPRING",
			sources: []string{al("10")}},
	)
	plan := Build(db, 2020, map[int]bool{999: true})

	if len(plan.New) != 1 {
		t.Fatalf("got %d series, want the two cours joined into one: %+v", len(plan.New), plan.New)
	}
	s := plan.New[0]
	if s.ID != "show" {
		t.Errorf("ID = %q, want the series named by the title they share", s.ID)
	}
	// SPRING precedes FALL, so the spring cour is season 1 whatever order
	// upstream listed them in.
	if len(s.Seasons) != 2 || s.Seasons[0] != "show-s1" || s.Seasons[1] != "show-s2" {
		t.Errorf("Seasons = %v, want [show-s1 show-s2]", s.Seasons)
	}
}

// TestBuildDeclaresExactlyWhatItsOwnTitleResolvesTo pins the rule that keeps a
// generated file buildable.
//
// resolveKind refuses a kind unless the number of upstream candidates equals
// the number of declared installments, and the candidates are the entries
// carrying the title the override declares. So what matters is not the year —
// it is that the slots match that title's pool. A sequel upstream lists under
// its own name resolves on its own with one slot; one listed under the shared
// series title must declare every installment sharing it, or nothing resolves.
//
// The first case leaves the sequel as a record separate from a predecessor the
// catalogue does not yet have. That is the orphan this import accepts
// deliberately: once this record exists, the year carrying the predecessor sees
// it as a merge candidate and reports it, so the five-year run converges rather
// than silently accumulating duplicates.
func TestBuildDeclaresExactlyWhatItsOwnTitleResolvesTo(t *testing.T) {
	t.Run("listed under its own name: one slot", func(t *testing.T) {
		db := load(t,
			entry{title: "Long Runner", typ: "TV", episodes: 12, year: 2017, season: "WINTER",
				sources: []string{al("20")}},
			entry{title: "Long Runner 2nd Season", typ: "TV", episodes: 12, year: 2020, season: "SPRING",
				sources: []string{al("21")}},
		)
		plan := Build(db, 2020, map[int]bool{999: true})
		if len(plan.New) != 1 {
			t.Fatalf("New = %+v, want one series", plan.New)
		}
		if got := plan.New[0].Seasons; len(got) != 1 {
			t.Errorf("declared %v, want one slot — only this entry carries its title", got)
		}
	})

	t.Run("listed under the shared series title: every slot", func(t *testing.T) {
		db := load(t,
			entry{title: "Shared Title", typ: "TV", episodes: 12, year: 2017, season: "WINTER",
				sources: []string{al("22")}},
			entry{title: "Shared Title 2nd Season", typ: "TV", episodes: 12, year: 2020, season: "SPRING",
				sources: []string{al("23")}, synonyms: []string{"Shared Title"}},
		)
		plan := Build(db, 2020, map[int]bool{999: true})
		if len(plan.New) != 1 {
			t.Fatalf("New = %+v, want one series", plan.New)
		}
		// The seed's native title is its own, which only it carries — so the
		// pool is one entry and one slot is right here too. What would break
		// is declaring one slot against a title the predecessor also carries,
		// which the next case covers.
		if got := plan.New[0].Seasons; len(got) != 1 {
			t.Errorf("declared %v, want one slot", got)
		}
	})

	t.Run("the seed's own title is shared: both slots", func(t *testing.T) {
		// Both entries carry "100万の命の上に俺は立っている", the kana title the
		// renderer picks, so declaring one season would leave two candidates
		// against one slot and the build would refuse. This is the real shape
		// that failed: 100-man no Inochi no Ue ni Ore wa Tatteiru.
		shared := "いちまんの命"
		db := load(t,
			entry{title: "Ichiman", typ: "TV", episodes: 12, year: 2020, season: "FALL",
				sources: []string{al("24")}, synonyms: []string{shared}},
			entry{title: "Ichiman 2nd Season", typ: "TV", episodes: 12, year: 2021, season: "SUMMER",
				sources: []string{al("25")}, synonyms: []string{shared}},
		)
		plan := Build(db, 2020, map[int]bool{999: true})
		if len(plan.New) != 1 {
			t.Fatalf("New = %+v, want one series", plan.New)
		}
		s := plan.New[0]
		if s.Original != shared {
			t.Errorf("Original = %q, want the shared kana title %q", s.Original, shared)
		}
		if len(s.Seasons) != 2 {
			t.Errorf("declared %v, want both seasons: two entries carry the declared title", s.Seasons)
		}
	})
}

// TestBuildRefusesSeveralFilmsSharingATitle keeps unbuildable files out of the
// tree. A movie node carries no number, so resolveKind declines the kind
// outright rather than pair several films against date-sorted candidates, and
// the installments are then left with nothing resolved.
func TestBuildRefusesSeveralFilmsSharingATitle(t *testing.T) {
	db := load(t,
		entry{title: "Double Bill", typ: "MOVIE", episodes: 1, year: 2020, season: "WINTER",
			sources: []string{al("30")}},
		entry{title: "Double Bill Part Two", typ: "MOVIE", episodes: 1, year: 2020, season: "SUMMER",
			sources: []string{al("31")}, synonyms: []string{"Double Bill"}},
	)
	plan := Build(db, 2020, map[int]bool{999: true})

	if len(plan.New) != 0 {
		t.Errorf("wrote %+v, want nothing: two films under one title cannot resolve", plan.New)
	}
	if len(plan.Unresolvable) != 1 {
		t.Fatalf("Unresolvable = %+v, want one entry explaining the refusal", plan.Unresolvable)
	}
	if !strings.Contains(plan.Unresolvable[0].Reason, "no number") {
		t.Errorf("reason = %q, want it to say why films cannot be ordered", plan.Unresolvable[0].Reason)
	}
}

// TestBuildRefusesAnUnairedInstallment covers the other count mismatch: the
// catalogue carries no announcements, but an unaired entry sharing the title is
// still a candidate the build counts, so the slots can never match.
func TestBuildRefusesAnUnairedInstallment(t *testing.T) {
	db := load(t,
		entry{title: "Airing Now", typ: "TV", episodes: 12, year: 2020, season: "SPRING",
			sources: []string{al("40")}},
		entry{title: "Airing Now Season 2", typ: "TV", episodes: 0, year: 2021,
			sources: []string{al("41")}, synonyms: []string{"Airing Now"}},
	)
	plan := Build(db, 2020, map[int]bool{999: true})

	if len(plan.New) != 0 {
		t.Errorf("wrote %+v, want nothing", plan.New)
	}
	if len(plan.Unresolvable) != 1 || !strings.Contains(plan.Unresolvable[0].Reason, "unreleased season") {
		t.Errorf("Unresolvable = %+v, want one entry naming the unreleased season", plan.Unresolvable)
	}
}

// TestBuildDropsAnUnreleasedFilmRatherThanTheSeries covers the other half of
// the announcement rule. A kind with no slots is skipped whatever the
// candidates are, so omitting the film resolves — and the series is carried
// with its season, which is the truth about it.
//
// The shape is real: "Majo no Tabitabi Movie" carries one episode and the
// UPCOMING status with no year, and an episode count alone let it into the
// catalogue as a work released in year zero.
func TestBuildDropsAnUnreleasedFilmRatherThanTheSeries(t *testing.T) {
	db := load(t,
		entry{title: "Majo no Tabitabi", typ: "TV", episodes: 12, year: 2020, season: "FALL",
			sources: []string{al("50")}},
		entry{title: "Majo no Tabitabi Movie", typ: "MOVIE", episodes: 1, season: "UNDEFINED",
			status: "UPCOMING", sources: []string{al("51")}, synonyms: []string{"Majo no Tabitabi"}},
	)
	plan := Build(db, 2020, map[int]bool{999: true})

	if len(plan.New) != 1 {
		t.Fatalf("New = %+v, want the series kept", plan.New)
	}
	s := plan.New[0]
	if len(s.Seasons) != 1 {
		t.Errorf("Seasons = %v, want the aired season", s.Seasons)
	}
	if len(s.Movies) != 0 {
		t.Errorf("Movies = %v, want none: the film is an announcement", s.Movies)
	}
}

func TestReleasedRejectsEveryShapeOfAnnouncement(t *testing.T) {
	db := load(t,
		entry{title: "No Episodes", typ: "TV", episodes: 0, year: 2020, sources: []string{al("60")}},
		entry{title: "Upcoming", typ: "TV", episodes: 12, year: 2027, status: "UPCOMING", sources: []string{al("61")}},
		entry{title: "Undated", typ: "TV", episodes: 12, season: "UNDEFINED", sources: []string{al("62")}},
		entry{title: "Aired", typ: "TV", episodes: 12, year: 2020, status: "FINISHED", sources: []string{al("63")}},
	)
	var got []string
	for a := range db.All() {
		if released(a) {
			got = append(got, a.Title)
		}
	}
	if len(got) != 1 || got[0] != "Aired" {
		t.Errorf("released() accepted %v, want only [Aired]", got)
	}
}

func TestNativeTitlePrefersTheFullJapaneseTitle(t *testing.T) {
	a := load(t, entry{
		title: "Seishun Buta Yarou wa Santa Claus no Yume wo Minai",
		typ:   "TV", episodes: 13, year: 2025,
		sources: []string{al("171046")},
		synonyms: []string{
			"AoButa",
			"Rascal Does Not Dream of Santa Claus",
			"青ブタ",         // abbreviation
			"青春ブタ野郎 大学生編", // arc label
			"青春ブタ野郎はサンタクロースの夢を見ない", // the one wanted
			"青春猪头少年不会梦到圣诞服女郎",      // Simplified Chinese: no kana
			"青春豬頭少年不會夢到聖誕服女郎",      // Traditional Chinese: no kana
		},
	})
	var e offlinedb.Anime
	for x := range a.All() {
		e = x
	}
	if got, want := NativeTitle(e), "青春ブタ野郎はサンタクロースの夢を見ない"; got != want {
		t.Errorf("NativeTitle() = %q, want %q", got, want)
	}
}

func TestNativeTitleFallsBackToTheEntryTitle(t *testing.T) {
	// No kana anywhere: upstream lists some works only under romanisations.
	// The entry's own title is then the only native handle there is, and it is
	// what the build will match on.
	db := load(t, entry{title: "Some Co-production", typ: "TV", episodes: 12, year: 2020,
		sources: []string{al("40")}, synonyms: []string{"Une Coproduction"}})
	var e offlinedb.Anime
	for x := range db.All() {
		e = x
	}
	if got := NativeTitle(e); got != "Some Co-production" {
		t.Errorf("NativeTitle() = %q, want the entry title", got)
	}
}

func TestSlug(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Seishun Buta Yarou wa Santa Claus no Yume wo Minai", "seishun-buta-yarou-wa-santa-claus-no-yume-wo-minai"},
		{"Kaguya-sama wa Kokurasetai?", "kaguya-sama-wa-kokurasetai"},
		{"Fate/stay night Movie: Heaven's Feel - III. Spring Song", "fate-stay-night-movie-heaven-s-feel-iii-spring-song"},
		{"Re:Zero kara Hajimeru Isekai Seikatsu", "re-zero-kara-hajimeru-isekai-seikatsu"},
		{"Healin' Good♡Precure", "healin-good-precure"},
		// Folded, not stripped: otherwise "Yarō" and "Yarou" become different
		// ids for the same series. Every vowel upstream romanises with a macron
		// or an accent, since the synonyms mix conventions freely.
		{"Seishun Buta Yarō", "seishun-buta-yaro"},
		{"Ākā Īē Ōō Ūū", "aka-ie-oo-uu"},
		{"âàá êèé îìí ôòó ûùú", "aaa-eee-iii-ooo-uuu"},
		{"Ăĕĭŏŭ", "aeiou"},
		{"  Leading and trailing  ", "leading-and-trailing"},
	} {
		if got := Slug(tc.in); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenderProducesTheAuthoredLayerShape(t *testing.T) {
	got := Render(Series{
		ID:        "show",
		Original:  "ショー",
		Romanized: "Show",
		Seasons:   []string{"show-s1", "show-s2"},
		Movies:    []string{"show-film"},
	}, 2020)

	for _, want := range []string{
		"# Authored override — 2020 catalogue import.",
		"  id: show\n",
		"    original: ショー\n",
		"    translations: { ja-Latn: Show }\n",
		"    - id: show-s1\n      number: 1\n",
		"    - id: show-s2\n      number: 2\n",
		"  movies:\n    - id: show-film\n",
		"numbered: [show]\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() missing %q; got:\n%s", want, got)
		}
	}
}

// TestRenderRoundTripsEveryAwkwardTitle asserts the property that matters:
// whatever a title contains, parsing the rendered file back gives the title
// again. Pinning which values get quotes would be asserting my reading of the
// YAML spec — and a first draft of this test did exactly that, and was wrong,
// because a plain scalar may carry interior double quotes.
func TestRenderRoundTripsEveryAwkwardTitle(t *testing.T) {
	titles := []string{
		"Plain Title",
		"ショー",
		"青春ブタ野郎はサンタクロースの夢を見ない",
		"Aikatsu! Memorial Stage: Kagayaki no Unit Cup", // colon-space ends a key
		"Re:Zero kara Hajimeru Isekai Seikatsu",         // colon, no space
		"- not a list item",                             // leading indicator
		"#not a comment",
		"? not a complex key",
		"has \"quotes\" inside",
		"back\\slash",
		"trailing colon:",
		"  padded  ",
		"100%",
		"&anchor",
		"*alias",
		"[bracketed]",
		"{braced}",
		"yes", // a bare YAML 1.1 boolean
		"null",
	}
	for _, title := range titles {
		out := Render(Series{ID: "x", Original: title, Romanized: title, Seasons: []string{"x-s1"}}, 2020)
		var got struct {
			Series struct {
				Titles struct {
					Original     string            `yaml:"original"`
					Translations map[string]string `yaml:"translations"`
				} `yaml:"titles"`
			} `yaml:"series"`
		}
		if err := yaml.Unmarshal([]byte(out), &got); err != nil {
			t.Errorf("title %q rendered unparseable YAML: %v\n%s", title, err, out)
			continue
		}
		if got.Series.Titles.Original != title {
			t.Errorf("original round-tripped to %q, want %q", got.Series.Titles.Original, title)
		}
		if got.Series.Titles.Translations["ja-Latn"] != title {
			t.Errorf("ja-Latn round-tripped to %q, want %q", got.Series.Titles.Translations["ja-Latn"], title)
		}
	}
}

// TestNativeTitleSkipsUpstreamsDisambiguators covers the two markers observed
// in the catalogue. The Date a Bullet case shipped: the series was written
// claiming to be Date A Live, because the other franchise's name is longer.
func TestNativeTitleSkipsUpstreamsDisambiguators(t *testing.T) {
	for _, tc := range []struct {
		name     string
		title    string
		synonyms []string
		want     string
	}{
		{
			name:  "a sibling franchise tagged (新アニメ)",
			title: "Date a Bullet",
			// Both are kana and the wrong one is longer, which is what made
			// length alone pick it.
			synonyms: []string{"デート・ア・バレット", "デート・ア・ライブ (新アニメ)"},
			want:     "デート・ア・バレット",
		},
		{
			name:     "a year tag",
			title:    "Black Clover",
			synonyms: []string{"ブラッククローバー", "ブラッククローバー (2017)"},
			want:     "ブラッククローバー",
		},
		{
			// Furigana is part of the name and the fullest form of it, so the
			// rule must not reach it.
			name:     "furigana is kept",
			title:    "Ao no Exorcist",
			synonyms: []string{"青の祓魔師", "青の祓魔師（エクソシスト）"},
			want:     "青の祓魔師（エクソシスト）",
		},
		{
			name:     "furigana with a season suffix is kept",
			title:    "BanG Dream! 3rd Season",
			synonyms: []string{"BanG Dream!（バンドリ！）第3期"},
			want:     "BanG Dream!（バンドリ！）第3期",
		},
		{
			// Every candidate tagged: the entry's own title is the only handle
			// left, and it is one the build can resolve.
			name:     "all candidates tagged falls back to the title",
			title:    "Ahare! Meisaku-kun",
			synonyms: []string{"あはれ! 名作くん (2020)"},
			want:     "Ahare! Meisaku-kun",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := load(t, entry{title: tc.title, typ: "TV", episodes: 12, year: 2020,
				sources: []string{al("70")}, synonyms: tc.synonyms})
			var e offlinedb.Anime
			for x := range db.All() {
				e = x
			}
			if got := NativeTitle(e); got != tc.want {
				t.Errorf("NativeTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}
