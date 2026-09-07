// Package offlinedb loads the manami-project anime-offline-database and indexes
// every entry by title and, where upstream carries one, by AniList id, so the
// build pipeline can fill facts (titles, season/year, episode counts) and
// cross-map external ids.
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

// Key identifies one entry within a loaded Database. The zero value names no
// entry, so an Anime built by hand is distinguishable from one a Database
// parsed.
//
// It is a handle, not an id. It is the entry's position in the file upstream
// published, so it means nothing to the next release: the offline database
// rolls, entries are added and removed, and every key after the change moves.
// Nothing that outlives the process may reference an entry by one — not an
// override, not a record in data/ — or a source refresh would silently
// re-point it at another work.
//
// It exists because half of upstream's entries have no AniList id and the
// build still has to tell them apart: to de-duplicate the candidates for one
// series, and to order two that aired in the same quarter.
type Key int

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

	// key is where this entry sits in the database that parsed it. Unexported
	// so it cannot be authored, decoded from JSON, or written back out.
	key Key
}

// Key returns the entry's handle within the database that parsed it, or 0 for
// an Anime no database produced.
func (a Anime) Key() Key { return a.key }

// Database is an indexed view of the offline database.
type Database struct {
	// entries holds every entry in the order upstream lists them; an entry's
	// position plus one is its Key. The indexes below hold keys into it rather
	// than entries of their own, so no entry is stored twice however many
	// names it is reachable under.
	entries []Anime
	// byAnilist indexes the entries upstream gives an AniList id — about half
	// of them — under it. It is the join key for everything the dataset
	// already carries, and the only handle that survives a source refresh.
	byAnilist map[int]Key
	// byTitle indexes every entry under its title and each of its synonyms, so
	// a work can be found by a name rather than by an id. A series has no
	// AniList id of its own — it spans several — so its own native title is the
	// only handle it has on the entries that make it up. It is also the only
	// handle there is on an entry upstream lists on no AniList page at all.
	//
	// Keys rather than entries: upstream carries tens of thousands of entries
	// with five to twenty synonyms each, so storing the struct in every bucket
	// would hold several hundred thousand copies of it live for as long as the
	// database is loaded. A key costs eight bytes and entries has the entry.
	byTitle map[string][]Key
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

// Parse reads an offline database from r and indexes every entry by title and
// synonym, plus the ones carrying an AniList id by that id.
//
// Nothing is dropped. Indexing only the AniList-bearing entries made the other
// half of upstream unreachable — 20,450 of its 40,921 entries under the pinned
// release, including 266 of the 643 TV and film entries it lists for 2025 —
// because a work absent from both indexes cannot be found by an id it does not
// have or by a title nothing recorded. Those works are on Anime News Network,
// MyAnimeList, AniDB or anisearch and are perfectly real; a build resolves one
// by title like any other and it simply ends up with no anilistId.
//
// Keeping them costs memory: the strings of the entries this used to discard
// now stay live for as long as the database does, roughly doubling what it
// holds. That is the price of a catalogue that can carry them at all.
func Parse(r io.Reader) (*Database, error) {
	var raw rawDatabase
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode offline database: %w", err)
	}
	db := &Database{
		entries:   make([]Anime, 0, len(raw.Data)),
		byAnilist: make(map[int]Key, len(raw.Data)),
		byTitle:   make(map[string][]Key, len(raw.Data)*4),
	}
	for _, a := range raw.Data {
		a.key = Key(len(db.entries) + 1)
		db.entries = append(db.entries, a)
		if id := a.AnilistID(); id != 0 {
			db.byAnilist[id] = a.key
		}
		for _, name := range append([]string{a.Title}, a.Synonyms...) {
			if name != "" {
				db.byTitle[name] = append(db.byTitle[name], a.key)
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
	k, ok := d.byAnilist[anilistID]
	if !ok {
		return Anime{}, false
	}
	return d.at(k), true
}

// at returns the entry a key names. A key comes from the database that issued
// it, so an out-of-range one is a bug rather than an input error.
func (d *Database) at(k Key) Anime { return d.entries[k-1] }

// Titled returns every entry carrying name as its title or one of its
// synonyms, in the order upstream lists them.
//
// That order is upstream's file order, which is fixed for a given database, so
// two runs agree — but it carries no meaning, and a caller that needs the
// entries ranked must sort them itself. Sorting here instead would be wasted
// work: the only caller merges several of these into a set keyed by entry and
// then orders the survivors by airing date, discarding whatever order it was
// given.
//
// An exact match rather than a prefix or a fold: upstream's synonym lists are
// long and multilingual, and anything looser turns "find this work" into "find
// works whose names look a bit like this", which is how a season of one show
// gets an id belonging to another. That reasoning is stronger, not weaker, now
// that the entries with no AniList id are indexed too: they are exactly the
// ones a wrong match could not later be caught out by a mismatched id.
func (d *Database) Titled(name string) []Anime {
	keys := d.byTitle[name]
	if len(keys) == 0 {
		return nil
	}
	out := make([]Anime, 0, len(keys))
	for _, k := range keys {
		out = append(out, d.at(k))
	}
	return out
}

// Len reports the number of indexed entries, which is every entry upstream
// carries.
func (d *Database) Len() int { return len(d.entries) }
