// Command proposeqids proposes a Wikidata QID for every series that has none,
// so a maintainer can review the matches and paste the good ones into the
// overrides.
//
// It is deliberately not a `builder` subcommand and never writes to the
// overrides. Resolving a title to an entity is a guess — of the first 80 this
// found, three were video games and one a disambiguation page — and the whole
// design of this dataset is that a guess does not become a committed fact
// without a human in between. NOTICE says the same thing about the cast:
// authored by hand, "with Wikidata used as a research aid". This is that aid.
//
// Usage:
//
//	go run ./cmd/proposeqids [-overrides config/overrides] [-limit 0]
//
// It prints TSV to stdout: series id, proposed QID, the Japanese title it
// matched, the English candidate, where the candidate came from, and a verdict
// saying whether the build would accept it.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/michael-freling/anime-metadata-db/builder/internal/fetch"
	"github.com/michael-freling/anime-metadata-db/builder/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// apiURL is the Wikidata action API. It is hardcoded rather than read from
// config.yaml because this tool is not part of a build: config.yaml pins the
// sources a build may read, and adding a research endpoint to it would imply
// the build reads one.
const apiURL = "https://www.wikidata.org/w/api.php"

// batchSize is the maximum number of titles per wbgetentities request.
const batchSize = 40

// disambiguationPage is the P31 value for a Wikimedia disambiguation page —
// never a work, and the single most common wrong match, since an anime title
// is exactly the kind of string that collides.
const disambiguationPage = "Q4167410"

func main() {
	dir := flag.String("overrides", "config/overrides", "path to the overrides directory")
	limit := flag.Int("limit", 0, "stop after this many unresolved series (0 = all)")
	flag.Parse()

	if err := run(*dir, *limit); err != nil {
		fmt.Fprintln(os.Stderr, "proposeqids:", err)
		os.Exit(1)
	}
}

func run(dir string, limit int) error {
	bundle, err := overrides.LoadDir(dir)
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
		fmt.Fprintln(os.Stderr, "every series already names a Wikidata work")
		return nil
	}

	client := &fetch.Client{HTTP: &http.Client{Timeout: 60 * time.Second}}
	ctx := context.Background()

	// Resolve by Japanese Wikipedia article title. The article title usually is
	// the original, which is why this works at all; where it is not — a
	// disambiguated page name, or a show too new to have an article — the
	// series simply goes unmatched and stays a manual lookup.
	byTitle := map[string]entity{}
	for _, batch := range chunk(targets, batchSize) {
		titles := make([]string, len(batch))
		for i, t := range batch {
			titles[i] = t.original
		}
		ents, err := lookup(ctx, client, titles)
		if err != nil {
			return err
		}
		for _, e := range ents {
			byTitle[e.jaWikiTitle] = e
		}
	}

	fmt.Println("series\tqid\tmatched_ja_title\ten_candidate\tsource\tverdict")
	var matched int
	for _, t := range targets {
		e, ok := byTitle[t.original]
		if !ok {
			fmt.Printf("%s\t-\t%s\t-\t-\tno jawiki article with this exact title; look it up by hand\n", t.id, t.original)
			continue
		}
		matched++
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", t.id, e.qid, t.original, or(e.candidate(), "-"), e.source(), e.verdict())
	}
	fmt.Fprintf(os.Stderr, "\n%d/%d series matched a Wikidata work. Review every row before pasting a qid into an override.\n",
		matched, len(targets))
	return nil
}

// entity is the part of a wbgetentities response this tool reads.
type entity struct {
	qid         string
	jaWikiTitle string
	label       string
	title       string // P1476 @ en
	instanceOf  []string
}

// candidate returns the English string the build would use, P1476 first.
func (e entity) candidate() string { return or(e.title, e.label) }

// source names where candidate came from, so a reviewer can weight it.
func (e entity) source() string {
	switch {
	case e.title != "":
		return "P1476"
	case e.label != "":
		return "label"
	}
	return "-"
}

// verdict says what the build would do with this match, so the reviewer reads
// a conclusion rather than re-deriving one per row.
func (e entity) verdict() string {
	for _, of := range e.instanceOf {
		if of == disambiguationPage {
			return "REJECT: a disambiguation page, not a work"
		}
	}
	if e.candidate() == "" {
		return "no English title or label; the qid is still worth authoring for later"
	}
	return "ok"
}

// lookup resolves Japanese Wikipedia article titles to their Wikidata entities,
// asking for the labels, title claims and P31 the verdict needs.
func lookup(ctx context.Context, client *fetch.Client, titles []string) ([]entity, error) {
	q := url.Values{}
	q.Set("action", "wbgetentities")
	q.Set("sites", "jawiki")
	q.Set("titles", strings.Join(titles, "|"))
	q.Set("props", "sitelinks|labels|claims")
	q.Set("languages", "en|ja")
	q.Set("sitefilter", "jawiki")
	q.Set("format", "json")

	body, err := client.Get(ctx, apiURL+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("wikidata lookup: %w", err)
	}
	var raw struct {
		Entities map[string]struct {
			ID       string `json:"id"`
			Missing  *string
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

	var out []entity
	for qid, re := range raw.Entities {
		if re.Missing != nil || strings.HasPrefix(qid, "-") {
			continue
		}
		e := entity{qid: qid, jaWikiTitle: re.SiteLink["jawiki"].Title, label: re.Labels["en"].Value}
		for _, st := range re.Claims["P1476"] {
			var v struct{ Text, Language string }
			if json.Unmarshal(st.MainSnak.DataValue.Value, &v) == nil && v.Language == "en" {
				e.title = v.Text
				break
			}
		}
		for _, st := range re.Claims["P31"] {
			var v struct{ ID string }
			if json.Unmarshal(st.MainSnak.DataValue.Value, &v) == nil {
				e.instanceOf = append(e.instanceOf, v.ID)
			}
		}
		if e.jaWikiTitle != "" {
			out = append(out, e)
		}
	}
	return out, nil
}

// or returns a, or b when a is empty.
func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// chunk splits a slice into batches of at most size.
func chunk[T any](in []T, size int) [][]T {
	var out [][]T
	for i := 0; i < len(in); i += size {
		end := min(i+size, len(in))
		out = append(out, in[i:end])
	}
	return out
}
