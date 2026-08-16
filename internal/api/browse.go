package api

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// WorkKind classifies a release: the three node types that map to something
// that actually aired or screened. A Franchise or Series is our grouping, not a
// release, so neither is a work — the same rule the coverage figures use.
type WorkKind int

// The work kinds.
const (
	WorkSeason WorkKind = iota
	WorkMovie
	WorkSpecial
)

// Work is one release flattened out of the Franchise -> Series -> Season
// hierarchy, carrying enough of its parent to be rendered on its own. Browsing
// by year or quarter is a query over releases, which the nested records cannot
// answer without the caller walking the whole tree.
type Work struct {
	Kind          WorkKind
	ID            string
	Titles        model.Title
	SeriesID      string
	SeriesTitles  model.Title
	Number        int // seasons only
	ReleaseDate   *model.Date
	ReleaseYear   int
	ReleaseSeason model.ReleaseSeason // seasons only
	Format        model.SpecialFormat // specials only
	EpisodeCount  int
	ExternalIDs   model.ExternalIDs
}

// WorkFilter narrows a work listing. A zero field matches everything, so the
// zero filter walks the entire dataset.
type WorkFilter struct {
	ReleaseYear   int
	ReleaseSeason model.ReleaseSeason
	Kind          *WorkKind
	SeriesID      string
}

// matches reports whether w satisfies every set field of f.
func (f WorkFilter) matches(w Work) bool {
	switch {
	case f.ReleaseYear != 0 && w.ReleaseYear != f.ReleaseYear:
		return false
	case f.ReleaseSeason != "" && w.ReleaseSeason != f.ReleaseSeason:
		return false
	case f.Kind != nil && w.Kind != *f.Kind:
		return false
	case f.SeriesID != "" && w.SeriesID != f.SeriesID:
		return false
	}
	return true
}

// indexWorks flattens every series into the works list and computes the
// per-entry catalog aggregates. It runs once, at load, so neither listing works
// nor listing the catalog has to walk the hierarchy per request.
func (s *Store) indexWorks() {
	byFranchise := make(map[string][]string, len(s.franchises))
	for id, ref := range s.seriesByID {
		if ref.franchiseID != "" {
			byFranchise[ref.franchiseID] = append(byFranchise[ref.franchiseID], id)
		}
	}

	// Walk s.entries rather than the map so the works list keeps the catalog's
	// deterministic order; ranging a map would shuffle it between runs.
	worksBySeries := make(map[string][]Work, len(s.seriesByID))
	for _, e := range s.entries {
		if e.Kind != EntrySeries {
			continue
		}
		ref := s.seriesByID[e.ID]
		worksBySeries[e.ID] = flattenSeries(ref.series)
		s.works = append(s.works, worksBySeries[e.ID]...)
	}

	// The dataset's own year span, over every work that carries one.
	for _, w := range s.works {
		if w.ReleaseYear == 0 {
			continue
		}
		if s.stats.EarliestReleaseYear == 0 || w.ReleaseYear < s.stats.EarliestReleaseYear {
			s.stats.EarliestReleaseYear = w.ReleaseYear
		}
		if w.ReleaseYear > s.stats.LatestReleaseYear {
			s.stats.LatestReleaseYear = w.ReleaseYear
		}
	}

	for i := range s.entries {
		e := &s.entries[i]
		ids := []string{e.ID}
		if e.Kind == EntryFranchise {
			ids = byFranchise[e.ID]
		}
		for _, id := range ids {
			for _, w := range worksBySeries[id] {
				e.Works++
				e.Episodes += w.EpisodeCount
				if w.ReleaseYear == 0 {
					continue
				}
				if e.FirstReleaseYear == 0 || w.ReleaseYear < e.FirstReleaseYear {
					e.FirstReleaseYear = w.ReleaseYear
				}
				if w.ReleaseYear > e.LatestReleaseYear {
					e.LatestReleaseYear = w.ReleaseYear
				}
			}
		}
	}
}

// flattenSeries turns one series' seasons, movies and specials into works, in
// the order they appear in the record.
func flattenSeries(series *model.Series) []Work {
	out := make([]Work, 0, len(series.Seasons)+len(series.Movies)+len(series.Specials))
	for _, s := range series.Seasons {
		out = append(out, Work{
			Kind: WorkSeason, ID: s.ID, Titles: s.Titles,
			SeriesID: series.ID, SeriesTitles: series.Titles,
			Number: s.Number, ReleaseDate: s.ReleaseDate, ReleaseYear: s.ReleaseYear,
			ReleaseSeason: s.ReleaseSeason, EpisodeCount: len(s.Episodes),
			ExternalIDs: s.ExternalIDs,
		})
	}
	for _, m := range series.Movies {
		out = append(out, Work{
			Kind: WorkMovie, ID: m.ID, Titles: m.Titles,
			SeriesID: series.ID, SeriesTitles: series.Titles,
			ReleaseDate: m.ReleaseDate, ReleaseYear: m.ReleaseYear,
			ExternalIDs: m.ExternalIDs,
		})
	}
	for _, sp := range series.Specials {
		out = append(out, Work{
			Kind: WorkSpecial, ID: sp.ID, Titles: sp.Titles,
			SeriesID: series.ID, SeriesTitles: series.Titles,
			ReleaseDate: sp.ReleaseDate, ReleaseYear: sp.ReleaseYear,
			Format: sp.Format, EpisodeCount: len(sp.Episodes),
			ExternalIDs: sp.ExternalIDs,
		})
	}
	return out
}

// Catalog returns one page of top-level entries, optionally restricted to a
// single kind, plus the total number matching. Entries keep the deterministic
// catalog order.
func (s *Store) Catalog(kind *EntryKind, token string, limit int) (Page[CatalogEntry], error) {
	matching := s.entries
	if kind != nil {
		matching = make([]CatalogEntry, 0, len(s.entries))
		for _, e := range s.entries {
			if e.Kind == *kind {
				matching = append(matching, e)
			}
		}
	}
	return paginate(matching, token, limit)
}

// Works returns one page of releases matching f, plus the total number
// matching. Works keep the deterministic catalog order.
func (s *Store) Works(f WorkFilter, token string, limit int) (Page[Work], error) {
	matching := make([]Work, 0, len(s.works))
	for _, w := range s.works {
		if f.matches(w) {
			matching = append(matching, w)
		}
	}
	return paginate(matching, token, limit)
}

// Page is one slice of a longer result set.
type Page[T any] struct {
	Items []T
	// NextToken is empty when Items reaches the end of the result set.
	NextToken string
	// Total counts every match, not just this page.
	Total int
}

// paginate slices in according to an opaque page token and a limit.
//
// The token currently encodes a plain offset, which is only sound because the
// dataset is immutable for the lifetime of a deployment — it is compiled into
// the binary, so there is no write path that could shift rows between two calls
// and make an offset skip or repeat an item. It is base64-encoded rather than
// sent as a bare integer so that clients cannot come to depend on the format:
// when the dataset outgrows an in-memory slice and this becomes a keyset
// cursor, the wire contract will not change.
func paginate[T any](in []T, token string, limit int) (Page[T], error) {
	offset, err := decodeCursor(token)
	if err != nil {
		return Page[T]{}, err
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	total := len(in)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	next := ""
	if end < total {
		next = encodeCursor(end)
	}
	return Page[T]{Items: in[offset:end], NextToken: next, Total: total}, nil
}

const cursorPrefix = "o:"

// encodeCursor renders an offset as an opaque page token.
func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + strconv.Itoa(offset)))
}

// decodeCursor reads an offset back out of a page token. An empty token is the
// start of the result set; anything unparseable is rejected rather than
// silently treated as the start, so a corrupted token surfaces as an error
// instead of an unexpected first page.
func decodeCursor(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("invalid page token")
	}
	s, ok := strings.CutPrefix(string(raw), cursorPrefix)
	if !ok {
		return 0, fmt.Errorf("invalid page token")
	}
	offset, err := strconv.Atoi(s)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid page token")
	}
	return offset, nil
}

// normalizeQuery lowercases and trims a search term. Every name and title
// search goes through it, so they cannot drift apart in whitespace or case
// handling.
func normalizeQuery(query string) string {
	return strings.TrimSpace(strings.ToLower(query))
}

// SearchPage returns catalog entries whose original or translated title
// contains query (case-insensitive), one page at a time. A blank query matches
// nothing. Results keep the deterministic catalog order.
func (s *Store) SearchPage(query, token string, limit int) (Page[CatalogEntry], error) {
	q := normalizeQuery(query)
	if q == "" {
		return Page[CatalogEntry]{}, nil
	}
	matching := make([]CatalogEntry, 0, len(s.entries))
	for _, e := range s.entries {
		if titleMatches(e.Titles, q) {
			matching = append(matching, e)
		}
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	return paginate(matching, token, limit)
}

// CharactersPage is Characters with a cursor, over the same shared filter.
// seriesID is assumed to have been validated by the caller; an unknown id
// yields an empty page. query narrows by name, using the same case-insensitive
// substring match the catalog search applies to titles, so searching behaves
// the same wherever a name is involved.
func (s *Store) CharactersPage(seriesID, query, token string, limit int) (Page[*model.Character], error) {
	all := s.charactersFor(seriesID)
	if q := normalizeQuery(query); q != "" {
		matching := make([]*model.Character, 0, len(all))
		for _, c := range all {
			if titleMatches(c.Names, q) {
				matching = append(matching, c)
			}
		}
		all = matching
	}
	return paginate(all, token, limit)
}

// StaffPage returns staff, or only those credited in language when it is
// non-empty, one page at a time in deterministic dataset order.
func (s *Store) StaffPage(language, query, token string, limit int) (Page[*model.Staff], error) {
	all := s.staff
	if language != "" {
		all = make([]*model.Staff, 0, len(s.staff))
		for _, st := range s.staff {
			for _, credit := range s.creditsByStaff[st.ID] {
				if credit.Language == language {
					all = append(all, st)
					break
				}
			}
		}
	}
	if q := normalizeQuery(query); q != "" {
		matching := make([]*model.Staff, 0, len(all))
		for _, st := range all {
			if titleMatches(st.Names, q) {
				matching = append(matching, st)
			}
		}
		all = matching
	}
	return paginate(all, token, limit)
}
