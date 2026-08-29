// Package wikidata loads names and titles from Wikidata, the one CC0 source the
// build may redistribute. It resolves QIDs to their multilingual labels — and,
// for the works a series names, to their P1476 title claims — via the
// wbgetentities API, caching the merged result for offline builds.
package wikidata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// batchSize is the maximum number of entity ids per wbgetentities request.
const batchSize = 50

// languages restricts fetched labels to the ones we store.
//
// "mul" is Wikidata's single label for values that do not vary by language —
// increasingly how a Latin-script personal name is recorded, in place of a
// per-language duplicate. Without it those entities come back with no label at
// all and the name is silently left empty.
const languages = "en|ja|mul"

// multilingual is the Wikidata language code for a label that applies to every
// language.
const multilingual = "mul"

// titleProperty is P1476 ("title"), the language-tagged title of a work.
//
// It is read in preference to the English label because the two answer
// different questions. A label names the *item* and follows Wikipedia's
// shortest-unambiguous-name convention, so 葬送のフリーレン is labelled
// "Frieren"; P1476 records the work's title, "Frieren: Beyond Journey's End".
// Only about a quarter of the works we reference carry P1476, so the label
// remains the fallback — but where both exist the claim is the better answer.
const titleProperty = "P1476"

// label is one localized label value.
type label struct {
	Language string `json:"language"`
	Value    string `json:"value"`
}

// monolingualText is the datavalue of a monolingual-text claim such as P1476:
// one string plus the language it is written in.
type monolingualText struct {
	Language string `json:"language"`
	Text     string `json:"text"`
}

// statement is the part of a wbgetentities claim we read: the main snak's
// monolingual-text value. Ranks, qualifiers and references are ignored.
type statement struct {
	MainSnak struct {
		DataValue struct {
			Value monolingualText `json:"value"`
		} `json:"datavalue"`
	} `json:"mainsnak"`
}

// claims holds every property unparsed, because a datavalue's shape depends on
// its property: P1476 is a monolingual-text object, an external identifier is a
// bare string, a date is another object again. Decoding the map into a single
// Go type asserts they all look alike, and the first entity carrying an
// identifier alongside its title fails the whole build. Only titleProperty is
// ever decoded, and only into the type that property actually has.
type claims map[string]json.RawMessage

// rawEntity is the wbgetentities shape of a single entity. Missing is present
// (as an empty string) when the entity id does not exist.
//
// Claims is what the API returns; Titles is the reduced form we cache. A full
// claims block is ~75x the size of a labels one and almost all of it is
// properties this build never reads, so FetchEntities distils Claims down to
// the P1476 values and drops the rest before the cache is written. Parse
// accepts either, so a cache written by an older builder still loads.
type rawEntity struct {
	ID      string            `json:"id"`
	Labels  map[string]label  `json:"labels,omitempty"`
	Claims  claims            `json:"claims,omitempty"`
	Titles  map[string]string `json:"titles,omitempty"`
	Missing *string           `json:"missing,omitempty"`
}

// titles reduces an entity's P1476 claims to a language-keyed map. A language
// claimed twice keeps the first value, wbgetentities returning statements in
// the order Wikidata ranks them.
func (r rawEntity) titles() map[string]string {
	if len(r.Titles) > 0 {
		return r.Titles
	}
	raw, ok := r.Claims[titleProperty]
	if !ok {
		return nil
	}
	var statements []statement
	if err := json.Unmarshal(raw, &statements); err != nil {
		// A shape we do not recognise is not worth failing a build over: the
		// English label still answers, and the report says the fill came from
		// it.
		return nil
	}
	out := make(map[string]string, len(statements))
	for _, st := range statements {
		v := st.MainSnak.DataValue.Value
		if v.Language == "" || v.Text == "" {
			continue
		}
		if _, seen := out[v.Language]; !seen {
			out[v.Language] = v.Text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// rawResponse is the wbgetentities response (and our cache file) shape.
type rawResponse struct {
	Entities map[string]rawEntity `json:"entities"`
}

// withoutDisambiguator drops the parenthetical Wikidata appends to a label when
// it would otherwise collide with another entity's: the Japanese label for Ian
// Sinclair is "イアン・シンクレア (声優)", where 声優 means "voice actor" and is
// there to tell him apart from someone else of the same name. It is an
// artifact of how Wikidata indexes labels, not part of anyone's name, so it is
// removed here — at the source that produces it — rather than in each consumer
// that would otherwise have to know about it.
//
// Leaving it in did more than store a wrong name: 声優 is kanji, so a label
// that is otherwise entirely katakana stopped looking like one, and the rule
// that picks a person's name over its Japanese rendering skipped him in
// silence.
func withoutDisambiguator(label string) string {
	// Brackets are matched by kind rather than by pair, because a label mixing
	// an ASCII "(" with a full-width "）" is exactly the sort of thing a
	// hand-edited label does, and leaving that one unstripped would put the
	// kanji back — reviving the bug this exists to fix, for one entity, in
	// silence. Whitespace is trimmed with unicode.IsSpace so the ideographic
	// space U+3000 counts.
	const (
		openers = "(（"
		closers = ")）"
	)
	for {
		trimmed := strings.TrimRightFunc(label, unicode.IsSpace)
		last, _ := utf8.DecodeLastRuneInString(trimmed)
		if !strings.ContainsRune(closers, last) {
			return label
		}
		i := strings.LastIndexAny(trimmed, openers)
		if i <= 0 {
			return label
		}
		name := strings.TrimRightFunc(trimmed[:i], unicode.IsSpace)
		if name == "" {
			return label
		}
		// Repeat: Wikidata occasionally stacks two of these, and stopping at
		// the outermost would leave a disambiguator-shaped remnant behind.
		label = name
	}
}

// Entity is a resolved Wikidata entity: its QID, labels by language code, and
// (for a work) its P1476 titles by language code.
type Entity struct {
	QID    string
	Labels map[string]string
	Titles map[string]string
}

// Entities is an indexed set of resolved entities.
type Entities struct {
	byQID map[string]Entity
}

// Parse reads a wbgetentities-shaped JSON document (a single
// {"entities": {...}} object) and indexes it by QID. Entities flagged
// "missing" are skipped. Titles are read from either the raw "claims" the API
// returns or the reduced "titles" the cache stores.
func Parse(r io.Reader) (*Entities, error) {
	var raw rawResponse
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode wikidata entities: %w", err)
	}
	e := &Entities{byQID: make(map[string]Entity, len(raw.Entities))}
	for qid, re := range raw.Entities {
		if re.Missing != nil {
			continue
		}
		labels := make(map[string]string, len(re.Labels))
		for lang, l := range re.Labels {
			labels[lang] = withoutDisambiguator(l.Value)
		}
		// A "mul" label stands in for a language that has none of its own.
		// Wikidata uses it when the value is the same everywhere, which is the
		// common case for a name written in Latin script.
		//
		// It fills en and deliberately not ja. The build takes the ja label as
		// Title.Original — "the original native-script form" — so a romanized
		// mul value would land there as if it were the native one, and would
		// also silence the "no Japanese label; original left empty" report that
		// is how a missing native name gets a human's attention. Better an
		// empty Original and a warning than a wrong Original and silence.
		if mul, ok := labels[multilingual]; ok {
			if labels["en"] == "" {
				labels["en"] = mul
			}
			delete(labels, multilingual)
		}
		e.byQID[qid] = Entity{QID: qid, Labels: labels, Titles: re.titles()}
	}
	return e, nil
}

// Load reads and parses a cached wikidata entities file.
func Load(path string) (*Entities, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open wikidata cache: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	return Parse(f)
}

// Lookup returns the entity for a QID.
func (e *Entities) Lookup(qid string) (Entity, bool) {
	ent, ok := e.byQID[qid]
	return ent, ok
}

// Len reports the number of indexed entities.
func (e *Entities) Len() int { return len(e.byQID) }

// Getter fetches the body of a URL.
type Getter func(ctx context.Context, url string) ([]byte, error)

// FetchEntities resolves QIDs via the wbgetentities API (batched), returning
// the merged cache bytes plus the parsed Entities. QIDs are de-duplicated and
// sorted for deterministic output.
//
// titleQIDs are the works whose P1476 title claims are wanted as well as their
// labels; they need not be a subset of qids. The two are requested separately
// because claims cannot be filtered by property server-side: a claims response
// is ~75x the size of a labels one, and only the works — a tenth of the QIDs
// this fetches, the rest being characters and staff — carry a title to read.
// Everything not in titleQIDs is fetched exactly as before.
func FetchEntities(ctx context.Context, get Getter, apiURL string, qids, titleQIDs []string) ([]byte, *Entities, error) {
	wantTitles := make(map[string]bool, len(titleQIDs))
	for _, id := range titleQIDs {
		wantTitles[id] = true
	}
	var labelsOnly, withTitles []string
	for _, id := range dedupeSorted(append(append([]string{}, qids...), titleQIDs...)) {
		if wantTitles[id] {
			withTitles = append(withTitles, id)
		} else {
			labelsOnly = append(labelsOnly, id)
		}
	}

	merged := make(map[string]rawEntity, len(labelsOnly)+len(withTitles))
	for _, group := range []struct {
		ids    []string
		claims bool
	}{{labelsOnly, false}, {withTitles, true}} {
		for _, batch := range chunk(group.ids, batchSize) {
			reqURL, err := buildURL(apiURL, batch, group.claims)
			if err != nil {
				return nil, nil, err
			}
			body, err := get(ctx, reqURL)
			if err != nil {
				return nil, nil, fmt.Errorf("fetch wikidata entities: %w", err)
			}
			var raw rawResponse
			if err := json.Unmarshal(body, &raw); err != nil {
				return nil, nil, fmt.Errorf("decode wikidata response: %w", err)
			}
			for qid, re := range raw.Entities {
				// Distil the claims to the one property we read, so the cache
				// holds a title rather than every statement Wikidata has about
				// the work.
				re.Titles, re.Claims = re.titles(), nil
				merged[qid] = re
			}
		}
	}
	out, err := json.MarshalIndent(rawResponse{Entities: merged}, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("encode wikidata cache: %w", err)
	}
	entities, err := Parse(strings.NewReader(string(out)))
	if err != nil {
		return nil, nil, err
	}
	return out, entities, nil
}

// buildURL constructs a wbgetentities request URL for a batch of QIDs, asking
// for claims as well as labels when the batch holds works we want titles for.
func buildURL(apiURL string, ids []string, claims bool) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("parse wikidata api url: %w", err)
	}
	q := u.Query()
	q.Set("action", "wbgetentities")
	props := "labels"
	if claims {
		props = "labels|claims"
	}
	q.Set("props", props)
	q.Set("languages", languages)
	q.Set("format", "json")
	q.Set("ids", strings.Join(ids, "|"))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// dedupeSorted returns the unique non-empty ids, sorted.
func dedupeSorted(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// chunk splits ids into batches of at most size.
func chunk(ids []string, size int) [][]string {
	var out [][]string
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, ids[i:end])
	}
	return out
}
