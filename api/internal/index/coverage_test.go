package index

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// The coverage page states, in three markdown tables, what the committed
// dataset actually holds. Nothing derived them, so they drifted silently: the
// cast table still claimed every voice-actor link was Japanese two releases
// after the English cast landed, and it took a reader noticing to catch it.
//
// This recomputes all three tables from data/ and compares them to the page. On
// a mismatch it prints the corrected markdown, so bringing the page back in
// line is a paste rather than a counting exercise.
//
// It lives here, in the package that already parses the whole dataset, so it
// runs in the ordinary test job with no extra wiring.
const coverageDoc = "web/content/docs/coverage.mdx"

func TestCoveragePageMatchesTheDataset(t *testing.T) {
	// The repository root, two modules up: data/ and web/ both live there.
	const root = "../../.."
	d, err := Load(os.DirFS(root))
	if err != nil {
		t.Fatalf("load the committed dataset: %v", err)
	}
	raw, err := os.ReadFile(root + "/" + coverageDoc)
	if err != nil {
		t.Fatalf("read %s: %v", coverageDoc, err)
	}
	doc := string(raw)

	for _, tc := range []struct {
		heading string
		want    []string
	}{
		{"## Works by year and season", worksByYear(d)},
		{"## Totals", totals(d)},
		{"## Cast", cast(d)},
	} {
		got, ok := tableAfter(doc, tc.heading)
		if !ok {
			t.Errorf("%s: no markdown table under this heading in %s", tc.heading, coverageDoc)
			continue
		}
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%s in %s does not match the dataset.\n\n--- the page says ---\n%s\n\n--- the data says ---\n%s\n",
				tc.heading, coverageDoc, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}

// tableAfter returns the first contiguous run of table lines following heading.
func tableAfter(doc, heading string) ([]string, bool) {
	i := strings.Index(doc, heading)
	if i < 0 {
		return nil, false
	}
	var table []string
	for _, line := range strings.Split(doc[i+len(heading):], "\n") {
		switch {
		case strings.HasPrefix(line, "|"):
			table = append(table, strings.TrimRight(line, " "))
		case len(table) > 0:
			return table, true
		}
	}
	return table, len(table) > 0
}

// count renders a table cell: an em dash for nothing, so an empty quarter reads
// as empty rather than as a zero someone has to interpret.
func count(n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprint(n)
}

// worksByYear renders the works-by-release-year table. A work is one Season,
// Movie or Special; movies and specials carry only a year, so they share the
// "Films & specials" column.
func worksByYear(d *Dataset) []string {
	type row struct{ winter, spring, summer, fall, film, episodes int }
	years := map[int]*row{}
	at := func(y int) *row {
		if years[y] == nil {
			years[y] = &row{}
		}
		return years[y]
	}
	for _, f := range d.files {
		f.rec.EachSeries(func(s *model.Series) {
			for _, sn := range s.Seasons {
				r := at(sn.ReleaseYear)
				switch sn.ReleaseSeason {
				case model.SeasonWinter:
					r.winter++
				case model.SeasonSpring:
					r.spring++
				case model.SeasonSummer:
					r.summer++
				case model.SeasonFall:
					r.fall++
				}
				r.episodes += len(sn.Episodes)
			}
			for _, m := range s.Movies {
				at(m.ReleaseYear).film++
			}
			for _, sp := range s.Specials {
				r := at(sp.ReleaseYear)
				r.film++
				r.episodes += len(sp.Episodes)
			}
		})
	}

	out := []string{
		"| Year | Winter | Spring | Summer | Fall | Films & specials | Total | Episodes |",
		"|---:|---:|---:|---:|---:|---:|---:|---:|",
	}
	sorted := make([]int, 0, len(years))
	for y := range years {
		sorted = append(sorted, y)
	}
	sort.Ints(sorted)
	var tot row
	for _, y := range sorted {
		r := years[y]
		works := r.winter + r.spring + r.summer + r.fall + r.film
		out = append(out, fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %d | %d |",
			y, count(r.winter), count(r.spring), count(r.summer), count(r.fall), count(r.film), works, r.episodes))
		tot.winter += r.winter
		tot.spring += r.spring
		tot.summer += r.summer
		tot.fall += r.fall
		tot.film += r.film
		tot.episodes += r.episodes
	}
	works := tot.winter + tot.spring + tot.summer + tot.fall + tot.film
	return append(out, fmt.Sprintf("| **Total** | **%d** | **%d** | **%d** | **%d** | **%d** | **%d** | **%d** |",
		tot.winter, tot.spring, tot.summer, tot.fall, tot.film, works, tot.episodes))
}

// totals renders the structural totals. A Franchise or Series is our grouping
// and is never a work, so the works row counts only the three node kinds.
func totals(d *Dataset) []string {
	var franchises, series, seasons, movies, specials, episodes int
	for _, f := range d.files {
		if f.rec.Franchise != nil {
			franchises++
		}
		f.rec.EachSeries(func(s *model.Series) {
			series++
			seasons += len(s.Seasons)
			movies += len(s.Movies)
			specials += len(s.Specials)
			for _, sn := range s.Seasons {
				episodes += len(sn.Episodes)
			}
			for _, sp := range s.Specials {
				episodes += len(sp.Episodes)
			}
		})
	}
	out := []string{
		"| | Count |",
		"|---|---:|",
		fmt.Sprintf("| Franchises | %d |", franchises),
		fmt.Sprintf("| Series | %d |", series),
		fmt.Sprintf("| Seasons | %d |", seasons),
		fmt.Sprintf("| Movies | %d |", movies),
	}
	// Specials get a row only once there are any: a row of zero would read as a
	// gap in the build rather than as a kind of node the catalogue has not hit.
	if specials > 0 {
		out = append(out, fmt.Sprintf("| Specials | %d |", specials))
	}
	return append(out,
		fmt.Sprintf("| Works (seasons + movies + specials) | %d |", seasons+movies+specials),
		fmt.Sprintf("| Episodes | %d |", episodes))
}

// recordCast is every character in one record. A franchise can hold cast at the
// franchise level (a character spanning its series) and inside each series, and
// counting only one of the two would undercount whichever the authors chose.
func recordCast(rec model.Record) []model.Character {
	if rec.Franchise == nil {
		return rec.Cast()
	}
	out := append([]model.Character{}, rec.Franchise.Characters...)
	for _, s := range rec.Franchise.Series {
		out = append(out, s.Characters...)
	}
	return out
}

// cast renders the R2 table: characters, the staff who voice them, and the
// links between them counted per language.
func cast(d *Dataset) []string {
	var series, characters, voiced int
	withCharacter := map[string]bool{}
	links := map[string]int{}
	for _, f := range d.files {
		f.rec.EachSeries(func(*model.Series) { series++ })
		for _, c := range recordCast(f.rec) {
			characters++
			cast := len(c.VoiceActors)
			for _, a := range c.Appearances {
				withCharacter[a.SeriesID] = true
				cast += len(a.VoiceActors)
				for _, va := range a.VoiceActors {
					links[va.Language]++
				}
			}
			for _, va := range c.VoiceActors {
				links[va.Language]++
			}
			if cast > 0 {
				voiced++
			}
		}
	}
	staff := len(d.staff)

	// Languages by how much of the dataset they cover, so the table leads with
	// the language it is actually built around.
	langs := make([]string, 0, len(links))
	for l := range links {
		langs = append(langs, l)
	}
	sort.Slice(langs, func(i, j int) bool {
		if links[langs[i]] != links[langs[j]] {
			return links[langs[i]] > links[langs[j]]
		}
		return langs[i] < langs[j]
	})
	total := 0
	parts := make([]string, len(langs))
	for i, l := range langs {
		total += links[l]
		parts[i] = fmt.Sprintf("%d `%s`", links[l], l)
	}

	return []string{
		"| | Count |",
		"|---|---:|",
		fmt.Sprintf("| Series with at least one character | %d of %d |", len(withCharacter), series),
		fmt.Sprintf("| Characters | %d |", characters),
		fmt.Sprintf("| …of those, with a voice actor | %d |", voiced),
		fmt.Sprintf("| Staff (voice actors) | %d |", staff),
		fmt.Sprintf("| Voice-actor links | %d — %s |", total, strings.Join(parts, ", ")),
	}
}
