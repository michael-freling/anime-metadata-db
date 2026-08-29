package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/michael-freling/anime-metadata-db/builder/internal/config"
	"github.com/michael-freling/anime-metadata-db/builder/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// proposalBatchSize is the maximum number of article titles per wbgetentities
// request.
const proposalBatchSize = 40

// disambiguationPage is the P31 value for a Wikimedia disambiguation page.
// Never a work, and the most common wrong match: an anime title is exactly the
// kind of string that collides with one.
const disambiguationPage = "Q4167410"

// workTypes are the P31 values a series may legitimately resolve to. The list
// is deliberately an allowlist rather than a denylist of things like video
// games: a match to something not named here is a match nobody has thought
// about, and the reviewer should see that rather than a quiet "ok".
var workTypes = map[string]string{
	"Q21198342":  "manga series",
	"Q104213567": "light novel series",
	"Q63952888":  "anime television series",
	"Q117467246": "anime television film",
	"Q1107":      "anime",
	"Q8274":      "manga",
	"Q5398426":   "television series",
	"Q11424":     "film",
	"Q8261":      "novel",
	"Q747381":    "light novel",
	"Q1667921":   "novel series",
	"Q17175676":  "webtoon",
	"Q562061":    "web novel",
	"Q1760610":   "comic book series",
	"Q838795":    "comic strip",
	"Q74262765":  "manhwa series",
	"Q11086742":  "anime television program",
	// A video game is a legitimate source work for this catalogue, not a wrong
	// match: Fate/stay night began as a 2004 visual novel, and AiPri and Arne
	// as arcade games that were animated afterwards. It stays on the list with
	// this note because "video game" reads like a mismatch until you know that.
	"Q7889": "video game (visual novel / arcade source work)",
}

// ProposeQIDs prints a reviewable Wikidata QID proposal for every series whose
// override names no work yet, and never writes one.
//
// Resolving a title to an entity is a guess: run over the whole catalogue it
// matched three video games and a disambiguation page. So this reports and a
// human decides, which is the same rule NOTICE states for the cast — authored
// by hand, "with Wikidata used as a research aid". This is that aid.
//
// It is a subcommand rather than a separate program because it needs exactly
// what a build needs — the overrides directory, the Wikidata endpoint from
// config.yaml, and the fetcher that sends the User-Agent Wikimedia requires.
// Standing it up separately meant duplicating all three.
func (a *App) ProposeQIDs(ctx context.Context, limit int) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	src, ok := cfg.Sources[config.SourceWikidata]
	if !ok || src.URL == "" {
		return fmt.Errorf("no wikidata source configured in %s", a.configPath())
	}
	bundle, err := overrides.LoadDir(filepath.Join(a.Dir, cfg.Settings.OverridesDir))
	if err != nil {
		return err
	}

	type target struct{ id, original string }
	var targets []target
	for _, o := range bundle.Series {
		o.EachSeries(func(s *model.Series) {
			if s.ExternalIDs.WikidataID != "" || s.Titles.Original == "" {
				return
			}
			targets = append(targets, target{s.ID, s.Titles.Original})
		})
	}
	if limit > 0 && len(targets) > limit {
		targets = targets[:limit]
	}
	if len(targets) == 0 {
		fmt.Fprintln(a.Out, "every series already names a Wikidata work")
		return nil
	}

	// Resolve by Japanese Wikipedia article title. The article title usually is
	// the original, which is why this works at all; where it is not — a
	// disambiguated page name, or a show too new to have an article — the
	// series goes unmatched and stays a manual lookup.
	byTitle := map[string]proposal{}
	for i := 0; i < len(targets); i += proposalBatchSize {
		end := min(i+proposalBatchSize, len(targets))
		titles := make([]string, 0, end-i)
		for _, t := range targets[i:end] {
			titles = append(titles, t.original)
		}
		found, err := a.lookupWorks(ctx, src.URL, titles)
		if err != nil {
			return err
		}
		for _, p := range found {
			byTitle[p.jaWikiTitle] = p
		}
	}

	fmt.Fprintln(a.Out, "series\tqid\tmatched_ja_title\ten_candidate\tsource\tverdict")
	var matched int
	for _, t := range targets {
		p, ok := byTitle[t.original]
		if !ok {
			fmt.Fprintf(a.Out, "%s\t-\t%s\t-\t-\tno jawiki article with this exact title; look it up by hand\n", t.id, t.original)
			continue
		}
		matched++
		fmt.Fprintf(a.Out, "%s\t%s\t%s\t%s\t%s\t%s\n",
			t.id, p.qid, t.original, orElse(p.candidate(), "-"), p.source(), p.verdict())
	}
	fmt.Fprintf(a.Out, "\n%d/%d series matched a Wikidata work. Review every row before pasting a qid into an override.\n",
		matched, len(targets))
	return nil
}

// proposal is one candidate match: the entity a title resolved to, and what
// the build would make of it.
type proposal struct {
	qid         string
	jaWikiTitle string
	label       string
	title       string // P1476 @ en
	instanceOf  []string
}

// candidate returns the English string the build would use, P1476 first.
func (p proposal) candidate() string { return orElse(p.title, p.label) }

// source names where candidate came from, so a reviewer can weight it.
func (p proposal) source() string {
	switch {
	case p.title != "":
		return "P1476"
	case p.label != "":
		return "label"
	}
	return "-"
}

// verdict states what the build would do with this match, so the reviewer reads
// a conclusion rather than re-deriving one per row.
func (p proposal) verdict() string {
	var known []string
	for _, of := range p.instanceOf {
		if of == disambiguationPage {
			return "REJECT: a disambiguation page, not a work"
		}
		if name, ok := workTypes[of]; ok {
			known = append(known, name)
		}
	}
	if len(known) == 0 {
		return fmt.Sprintf("CHECK: not a recognised kind of work (P31 %s)", strings.Join(p.instanceOf, ", "))
	}
	if p.candidate() == "" {
		return fmt.Sprintf("%s, but no English title or label; the qid is still worth authoring", known[0])
	}
	return "ok — " + known[0]
}

// lookupWorks resolves Japanese Wikipedia article titles to their Wikidata
// entities, asking for the labels, title claims and P31 the verdict needs.
func (a *App) lookupWorks(ctx context.Context, apiURL string, titles []string) ([]proposal, error) {
	q := url.Values{}
	q.Set("action", "wbgetentities")
	q.Set("sites", "jawiki")
	q.Set("titles", strings.Join(titles, "|"))
	q.Set("props", "sitelinks|labels|claims")
	q.Set("languages", "en|ja")
	q.Set("sitefilter", "jawiki")
	q.Set("format", "json")

	body, err := a.Fetcher.Get(ctx, apiURL+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("wikidata lookup: %w", err)
	}
	var raw struct {
		Entities map[string]struct {
			Missing  *string                           `json:"missing"`
			Labels   map[string]struct{ Value string } `json:"labels"`
			SiteLink map[string]struct{ Title string } `json:"sitelinks"`
			Claims   map[string][]struct {
				MainSnak struct {
					DataValue struct {
						Value json.RawMessage `json:"value"`
					} `json:"datavalue"`
				} `json:"mainsnak"`
			} `json:"claims"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode wikidata response: %w", err)
	}

	var out []proposal
	for qid, re := range raw.Entities {
		// A title with no entity comes back keyed by a negative placeholder id.
		if re.Missing != nil || strings.HasPrefix(qid, "-") {
			continue
		}
		p := proposal{qid: qid, jaWikiTitle: re.SiteLink["jawiki"].Title, label: re.Labels["en"].Value}
		for _, st := range re.Claims["P1476"] {
			var v struct{ Text, Language string }
			if json.Unmarshal(st.MainSnak.DataValue.Value, &v) == nil && v.Language == "en" {
				p.title = v.Text
				break
			}
		}
		for _, st := range re.Claims["P31"] {
			var v struct{ ID string }
			if json.Unmarshal(st.MainSnak.DataValue.Value, &v) == nil && v.ID != "" {
				p.instanceOf = append(p.instanceOf, v.ID)
			}
		}
		if p.jaWikiTitle != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// orElse returns a, or b when a is empty.
func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
