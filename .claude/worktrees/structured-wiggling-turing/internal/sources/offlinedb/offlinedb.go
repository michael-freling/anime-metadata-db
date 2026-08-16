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
	for _, s := range sources {
		if !containsHost(s, host) {
			continue
		}
		m := idPattern.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		id, err := strconv.Atoi(m[1])
		if err == nil {
			return id
		}
	}
	return 0
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

// Parse reads an offline database from r and indexes it by AniList id. Entries
// without an AniList id are skipped (they cannot be referenced by overrides).
func Parse(r io.Reader) (*Database, error) {
	var raw rawDatabase
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode offline database: %w", err)
	}
	db := &Database{byAnilist: make(map[int]Anime, len(raw.Data))}
	for _, a := range raw.Data {
		if id := a.AnilistID(); id != 0 {
			db.byAnilist[id] = a
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

// Len reports the number of indexed entries.
func (d *Database) Len() int { return len(d.byAnilist) }
