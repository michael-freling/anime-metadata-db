// Package animelists parses the Anime-Lists project's anime-list.xml (AniDB ↔
// TVDB season offsets, used to cross-check absolute numbering) and
// anime-movieset-list.xml (movie-set grouping). Both are keyed by AniDB id.
package animelists

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
)

// Mapping is one <anime> element of anime-list.xml: an AniDB node mapped onto a
// TVDB entry with a default season and an absolute-episode offset.
type Mapping struct {
	AnidbID           int
	TvdbID            int
	DefaultTvdbSeason int
	EpisodeOffset     int
}

// AnimeList is the indexed anime-list.xml.
type AnimeList struct {
	byAnidb map[int]Mapping
}

// rawAnimeList is the on-disk XML shape of anime-list.xml.
type rawAnimeList struct {
	Anime []struct {
		AnidbID           int    `xml:"anidbid,attr"`
		TvdbID            string `xml:"tvdbid,attr"`
		DefaultTvdbSeason string `xml:"defaulttvdbseason,attr"`
		EpisodeOffset     int    `xml:"episodeoffset,attr"`
	} `xml:"anime"`
}

// ParseAnimeList reads and indexes anime-list.xml from r.
func ParseAnimeList(r io.Reader) (*AnimeList, error) {
	var raw rawAnimeList
	if err := xml.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode anime-list.xml: %w", err)
	}
	al := &AnimeList{byAnidb: make(map[int]Mapping, len(raw.Anime))}
	for _, a := range raw.Anime {
		if a.AnidbID == 0 {
			continue
		}
		// tvdbid is sometimes a non-numeric placeholder ("unknown", "movie");
		// treat those as 0 rather than failing the whole parse.
		tvdbID, _ := strconv.Atoi(a.TvdbID)
		season, _ := strconv.Atoi(a.DefaultTvdbSeason)
		al.byAnidb[a.AnidbID] = Mapping{
			AnidbID:           a.AnidbID,
			TvdbID:            tvdbID,
			DefaultTvdbSeason: season,
			EpisodeOffset:     a.EpisodeOffset,
		}
	}
	return al, nil
}

// LoadAnimeList reads and parses anime-list.xml from path.
func LoadAnimeList(path string) (*AnimeList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open anime-list.xml: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	return ParseAnimeList(f)
}

// Offset returns the absolute-episode offset for an AniDB id, if present.
func (a *AnimeList) Offset(anidbID int) (Mapping, bool) {
	m, ok := a.byAnidb[anidbID]
	return m, ok
}

// Len reports the number of indexed mappings.
func (a *AnimeList) Len() int { return len(a.byAnidb) }

// MovieSet is one <set> of anime-movieset-list.xml: a named group of AniDB
// movie ids that belong together.
type MovieSet struct {
	Name     string
	AnidbIDs []int
}

// MovieSetList is the indexed anime-movieset-list.xml.
type MovieSetList struct {
	sets    []MovieSet
	byAnidb map[int]int // anidb id -> index into sets
}

// rawMovieSetList is the on-disk XML shape of anime-movieset-list.xml.
type rawMovieSetList struct {
	Set []struct {
		Name  string `xml:"name,attr"`
		Anime []struct {
			AnidbID int `xml:"anidbid,attr"`
		} `xml:"anime"`
	} `xml:"set"`
}

// ParseMovieSetList reads and indexes anime-movieset-list.xml from r.
func ParseMovieSetList(r io.Reader) (*MovieSetList, error) {
	var raw rawMovieSetList
	if err := xml.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode anime-movieset-list.xml: %w", err)
	}
	msl := &MovieSetList{byAnidb: make(map[int]int)}
	for _, s := range raw.Set {
		set := MovieSet{Name: s.Name}
		for _, a := range s.Anime {
			if a.AnidbID == 0 {
				continue
			}
			set.AnidbIDs = append(set.AnidbIDs, a.AnidbID)
		}
		if len(set.AnidbIDs) == 0 {
			continue
		}
		idx := len(msl.sets)
		msl.sets = append(msl.sets, set)
		for _, id := range set.AnidbIDs {
			msl.byAnidb[id] = idx
		}
	}
	return msl, nil
}

// LoadMovieSetList reads and parses anime-movieset-list.xml from path.
func LoadMovieSetList(path string) (*MovieSetList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open anime-movieset-list.xml: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	return ParseMovieSetList(f)
}

// SetFor returns the movie set an AniDB id belongs to, if any.
func (m *MovieSetList) SetFor(anidbID int) (MovieSet, bool) {
	idx, ok := m.byAnidb[anidbID]
	if !ok {
		return MovieSet{}, false
	}
	return m.sets[idx], true
}

// Len reports the number of movie sets.
func (m *MovieSetList) Len() int { return len(m.sets) }
