// Package offlinedb loads the manami-project anime-offline-database and indexes
// its entries by AniList id so the build pipeline can fill facts (titles,
// season/year, episode counts) and cross-map external ids.
package offlinedb

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
)

// MediaType mirrors anime-offline-database's "type" field.
type MediaType string

// The recognised media types.
const (
	TypeTV      MediaType = "TV"
	TypeMovie   MediaType = "MOVIE"
	TypeOVA     MediaType = "OVA"
	TypeONA     MediaType = "ONA"
	TypeSpecial MediaType = "SPECIAL"
	TypeUnknown MediaType = "UNKNOWN"
)

// AnimeSeason is the upstream airing-season block.
type AnimeSeason struct {
	Season string `json:"season"`
	Year   int    `json:"year"`
}

// Anime is one entry of the offline database.
type Anime struct {
	Sources      []string    `json:"sources"`
	Title        string      `json:"title"`
	Type         MediaType   `json:"type"`
	Episodes     int         `json:"episodes"`
	Status       string      `json:"status"`
	AnimeSeason  AnimeSeason `json:"animeSeason"`
	Synonyms     []string    `json:"synonyms"`
	RelatedAnime []string    `json:"relatedAnime"`
}

// Database is an indexed view of the offline database.
type Database struct {
	byAnilist map[int]Anime
	// byTitle indexes every entry under its title and each of its synonyms, so
	// a work can be found by a name rather than by an id. A series has no
	// AniList id of its own — it spans several — so its own native title is the
	// only handle it has on the entries that make it up.
	//
	// Ids rather than entries: upstream carries tens of thousands of entries
	// with five to twenty synonyms each, so storing the struct in every bucket
	// would hold several hundred thousand copies of it live for as long as the
	// database is loaded. The id costs eight bytes and byAnilist already has
	// the entry.
	byTitle map[string][]int
}

// rawDatabase is the on-disk JSON shape.
type rawDatabase struct {
	Data []Anime `json:"data"`
}

// idPattern extracts the trailing numeric id from a provider URL such as
// https://anilist.co/anime/101922 or https://anidb.net/anime/14353.
var idPattern = regexp.MustCompile(`/(\d+)/?$`)

// providerHost is the host substring identifying each external provider.
const (
	hostAnilist = "anilist.co/anime/"
	hostAnidb   = "anidb.net/anime/"
	hostMyAL    = "myanimelist.net/anime/"
	hostKitsu   = "kitsu.app/anime/"
)

// providerID returns the numeric id for the given provider host within the
// entry's sources, or 0 if absent.
func providerID(sources []string, host string) int {
	if ids := providerIDs(sources, host); len(ids) > 0 {
		return ids[0]
	}
	return 0
}

// providerIDs returns every numeric id for the given provider host, in the
// order the urls are listed. A url for another provider, or one whose tail is
// not a number, is skipped: upstream mixes providers in both sources and
// relatedAnime, and only the ones this dataset joins on can be compared.
func providerIDs(sources []string, host string) []int {
	var out []int
	for _, s := range sources {
		if !containsHost(s, host) {
			continue
		}
		m := idPattern.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		if id, err := strconv.Atoi(m[1]); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// containsHost reports whether url contains the host substring.
func containsHost(url, host string) bool {
	for i := 0; i+len(host) <= len(url); i++ {
		if url[i:i+len(host)] == host {
			return true
		}
	}
	return false
}

// AnilistID returns the entry's AniList id, or 0 if it has none.
func (a Anime) AnilistID() int { return providerID(a.Sources, hostAnilist) }

// AnidbID returns the entry's AniDB id, or 0 if it has none.
func (a Anime) AnidbID() int { return providerID(a.Sources, hostAnidb) }

// MyAnimeListID returns the entry's MyAnimeList id, or 0 if it has none.
func (a Anime) MyAnimeListID() int { return providerID(a.Sources, hostMyAL) }

// KitsuID returns the entry's Kitsu id, or 0 if it has none.
func (a Anime) KitsuID() int { return providerID(a.Sources, hostKitsu) }

// RelatedAnilistIDs returns the AniList ids of the entries upstream links this
// one to — the other installments of the same work, plus its spin-offs and
// adaptations. Entries related only through a provider this dataset does not
// join on are skipped, since there is nothing to compare them against.
//
// Unlike the single-id accessors this returns every match rather than the
// first: the point of relatedAnime is the whole set.
func (a Anime) RelatedAnilistIDs() []int { return providerIDs(a.RelatedAnime, hostAnilist) }

// Parse reads an offline database from r and indexes it by AniList id. Entries
// without an AniList id are skipped (they cannot be referenced by overrides).
func Parse(r io.Reader) (*Database, error) {
	var raw rawDatabase
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode offline database: %w", err)
	}
	db := &Database{
		byAnilist: make(map[int]Anime, len(raw.Data)),
		byTitle:   make(map[string][]int, len(raw.Data)*4),
	}
	for _, a := range raw.Data {
		if id := a.AnilistID(); id != 0 {
			db.byAnilist[id] = a
			for _, name := range append([]string{a.Title}, a.Synonyms...) {
				if name != "" {
					db.byTitle[name] = append(db.byTitle[name], id)
				}
			}
		}
	}
	return db, nil
}

// Load reads and parses an offline database file from path.
func Load(path string) (*Database, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open offline database: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	return Parse(f)
}

// Lookup returns the entry for an AniList id.
func (d *Database) Lookup(anilistID int) (Anime, bool) {
	a, ok := d.byAnilist[anilistID]
	return a, ok
}

// Titled returns every entry carrying name as its title or one of its
// synonyms, in the order upstream lists them.
//
// That order is upstream's file order, which is fixed for a given database, so
// two runs agree — but it carries no meaning, and a caller that needs the
// entries ranked must sort them itself. Sorting here instead would be wasted
// work: the only caller merges several of these into a set keyed by id and then
// orders the survivors by airing date, discarding whatever order it was given.
//
// An exact match rather than a prefix or a fold: upstream's synonym lists are
// long and multilingual, and anything looser turns "find this work" into "find
// works whose names look a bit like this", which is how a season of one show
// gets an id belonging to another.
func (d *Database) Titled(name string) []Anime {
	ids := d.byTitle[name]
	if len(ids) == 0 {
		return nil
	}
	out := make([]Anime, 0, len(ids))
	for _, id := range ids {
		out = append(out, d.byAnilist[id])
	}
	return out
}

// Len reports the number of indexed entries.
func (d *Database) Len() int { return len(d.byAnilist) }
