// Package api is the read-only Connect service over the committed anime
// dataset. It is deliberately separate from internal/builder: the builder
// writes data/, this package serves it. The service implementation converts the
// internal/model records into the generated anime.v1 protobuf messages.
package api

import (
	"fmt"
	"io/fs"
	"path"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/michael-freling/anime-metadata-db/internal/index"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// The types the service works in are the index's: listing a browse page and
// reading the index are the same operation, so there is nothing for this
// package to redefine.
type (
	// CatalogEntry is a searchable top-level node.
	CatalogEntry = index.CatalogEntry
	// EntryKind classifies a catalog entry.
	EntryKind = index.EntryKind
	// Work is one release flattened out of the hierarchy.
	Work = index.Work
	// WorkKind classifies a release.
	WorkKind = index.WorkKind
	// WorkFilter narrows a work listing.
	WorkFilter = index.WorkFilter
	// Stats summarizes the loaded dataset.
	Stats = index.Stats
	// StaffCredit is one role a staff member is cast in.
	StaffCredit = index.StaffCredit
	// Page is one slice of a longer result set.
	Page[T any] = index.Page[T]
)

// The catalog entry kinds.
const (
	EntryFranchise = index.EntryFranchise
	EntrySeries    = index.EntrySeries
)

// The work kinds.
const (
	WorkSeason  = index.WorkSeason
	WorkMovie   = index.WorkMovie
	WorkSpecial = index.WorkSpecial
)

// Store answers requests from the prebuilt listing index, reading a record file
// only when a request names a single id.
//
// Listing, searching and browsing touch the index alone — see internal/index
// for why that matters — so the only YAML parsed on a request path is the one
// file behind a detail page.
//
// It is safe for concurrent use.
type Store struct {
	ix   *index.Index
	fsys fs.FS

	// Detail requests come in bursts against the same file: a series page reads
	// the series and then its cast, and a page of one series' cast is one file
	// repeated. A small cache turns those into a single parse without letting
	// the process drift back towards holding the whole dataset.
	mu     sync.Mutex
	cache  map[string]*model.Record
	recent []string
}

// recordCacheSize bounds the parsed-record cache. It is deliberately small:
// the cache exists to collapse the handful of reads one page makes, not to
// keep the dataset resident.
const recordCacheSize = 16

// NewStore returns a store serving ix, reading record files from fsys.
//
// fsys is the dataset root — the filesystem holding data/series/*.yaml — and is
// read from lazily, so nothing is parsed here.
func NewStore(ix *index.Index, fsys fs.FS) *Store {
	return &Store{ix: ix, fsys: fsys, cache: map[string]*model.Record{}}
}

// NewStoreFromDataset builds an index from the YAML in fsys and returns a store
// over it, for callers with no prebuilt index — tests, and tools that read a
// dataset they have just written.
//
// It parses the whole dataset, which is what the committed index exists to
// avoid. The server must not use it.
func NewStoreFromDataset(fsys fs.FS) (*Store, error) {
	ix, err := index.Build(fsys)
	if err != nil {
		return nil, err
	}
	return NewStore(ix, fsys), nil
}

// record parses one data/series file, or returns the cached parse.
func (s *Store) record(file string) (*model.Record, error) {
	s.mu.Lock()
	if rec, ok := s.cache[file]; ok {
		s.mu.Unlock()
		return rec, nil
	}
	s.mu.Unlock()

	raw, err := fs.ReadFile(s.fsys, path.Join("data/series", file))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file, err)
	}
	rec := &model.Record{}
	if err := yaml.Unmarshal(raw, rec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-check: a concurrent reader may have parsed the same file. Keeping the
	// first copy means two callers holding the same record see the same pointer.
	if existing, ok := s.cache[file]; ok {
		return existing, nil
	}
	if len(s.recent) >= recordCacheSize {
		delete(s.cache, s.recent[0])
		s.recent = s.recent[1:]
	}
	s.cache[file] = rec
	s.recent = append(s.recent, file)
	return rec, nil
}

// A note on drift, since the two halves behave differently on purpose.
//
// Listings answer from the index alone and never open a record, so they cannot
// notice that data/index.tsv has fallen out of step with data/series — they
// would serve stale titles and counts happily. The detail reads do notice,
// because they open the file the index names and check the record is the one it
// promised, and they return an error rather than a not-found.
//
// That asymmetry is the point: making listings verify would mean reading every
// record they list, which is the cost this whole package exists to avoid. Drift
// is prevented rather than detected — `make index-check` regenerates the index
// in CI and fails on any difference — and the detail path's check is the
// backstop for the case where prevention was bypassed.

// Stats returns the dataset summary recorded in the index.
func (s *Store) Stats() Stats { return s.ix.Stats() }

// FranchisesPage returns one page of franchises, fully expanded.
//
// This is the one listing that reads record files, because a franchise response
// embeds its whole tree. It is paginated for that reason: unbounded, it would
// be a request to parse the entire dataset.
func (s *Store) FranchisesPage(token string, limit int) (Page[*model.Franchise], error) {
	refs := s.ix.Franchises()
	p, err := index.Paginate(refs, token, limit)
	if err != nil {
		return Page[*model.Franchise]{}, err
	}
	out := Page[*model.Franchise]{NextToken: p.NextToken, Total: p.Total}
	for _, ref := range p.Items {
		f, ok, err := s.franchiseIn(ref.File, ref.ID)
		if err != nil {
			return Page[*model.Franchise]{}, err
		}
		if ok {
			out.Items = append(out.Items, f)
		}
	}
	return out, nil
}

// Franchise returns the franchise with the given id, or false if none exists.
func (s *Store) Franchise(id string) (*model.Franchise, bool, error) {
	file, ok := s.ix.Franchise(id)
	if !ok {
		return nil, false, nil
	}
	return s.franchiseIn(file, id)
}

// franchiseIn reads the franchise with the given id out of one record file.
func (s *Store) franchiseIn(file, id string) (*model.Franchise, bool, error) {
	rec, err := s.record(file)
	if err != nil {
		return nil, false, err
	}
	if rec.Franchise == nil || rec.Franchise.ID != id {
		// The index named this file for this id, so a miss means the two have
		// drifted — the index is stale relative to data/.
		return nil, false, fmt.Errorf("%s: index names franchise %q, which the record does not contain", file, id)
	}
	return rec.Franchise, true, nil
}

// Series returns the series with the given id and its owning franchise id
// (empty for a standalone series), or false if none exists.
func (s *Store) Series(id string) (*model.Series, string, bool, error) {
	file, franchiseID, ok := s.ix.Series(id)
	if !ok {
		return nil, "", false, nil
	}
	rec, err := s.record(file)
	if err != nil {
		return nil, "", false, err
	}
	var found *model.Series
	rec.EachSeries(func(series *model.Series) {
		if series.ID == id {
			found = series
		}
	})
	if found == nil {
		return nil, "", false, fmt.Errorf("%s: index names series %q, which the record does not contain", file, id)
	}
	return found, franchiseID, true, nil
}

// SeriesExists reports whether a series id is known, without reading its file.
func (s *Store) SeriesExists(id string) bool {
	_, _, ok := s.ix.Series(id)
	return ok
}

// Character returns the character with the given id, or false if none exists.
func (s *Store) Character(id string) (*model.Character, bool, error) {
	file, ok := s.ix.Character(id)
	if !ok {
		return nil, false, nil
	}
	c, err := s.characterIn(file, id)
	if err != nil {
		return nil, false, err
	}
	return c, true, nil
}

// characterIn reads one character out of a record file.
func (s *Store) characterIn(file, id string) (*model.Character, error) {
	rec, err := s.record(file)
	if err != nil {
		return nil, err
	}
	cast := rec.Cast()
	for i := range cast {
		if cast[i].ID == id {
			return &cast[i], nil
		}
	}
	return nil, fmt.Errorf("%s: index names character %q, which the record does not contain", file, id)
}

// Staff returns the staff member with the given id, or false if none exists.
// Staff records are small enough to live in the index in full, so this reads no
// file.
func (s *Store) Staff(id string) (*model.Staff, bool) { return s.ix.Staff(id) }

// StaffCredits returns every character the staff member is cast as, in
// deterministic dataset order (nil for an unknown or uncredited id).
func (s *Store) StaffCredits(staffID string) []StaffCredit { return s.ix.Credits(staffID) }

// Catalog returns one page of top-level entries, optionally restricted to a
// single kind.
func (s *Store) Catalog(kind *EntryKind, token string, limit int) (Page[CatalogEntry], error) {
	return s.ix.Catalog(kind, token, limit)
}

// Works returns one page of releases matching f.
func (s *Store) Works(f WorkFilter, token string, limit int) (Page[Work], error) {
	return s.ix.Works(f, token, limit)
}

// SearchPage returns catalog entries whose original or translated title
// contains query (case-insensitive), one page at a time.
func (s *Store) SearchPage(query, token string, limit int) (Page[CatalogEntry], error) {
	return s.ix.Search(query, token, limit)
}

// StaffPage returns staff, or only those credited in language when it is
// non-empty, one page at a time.
func (s *Store) StaffPage(language, query, token string, limit int) (Page[*model.Staff], error) {
	return s.ix.StaffPage(language, query, token, limit)
}

// CharactersPage returns one page of the cast of seriesID, or of the whole cast
// when seriesID is empty, narrowed by query.
//
// Only the page's own characters are read from disk, and a page drawn from one
// series is one file.
func (s *Store) CharactersPage(seriesID, query, token string, limit int) (Page[*model.Character], error) {
	refs, err := s.ix.Characters(seriesID, query, token, limit)
	if err != nil {
		return Page[*model.Character]{}, err
	}
	out := Page[*model.Character]{NextToken: refs.NextToken, Total: refs.Total}
	for _, ref := range refs.Items {
		c, err := s.characterIn(ref.File, ref.ID)
		if err != nil {
			return Page[*model.Character]{}, err
		}
		out.Items = append(out.Items, c)
	}
	return out, nil
}

// Characters returns the cast of seriesID, or the whole cast when seriesID is
// empty. limit caps the count; a non-positive limit applies the default.
//
// It shares the index's definition of a series' cast with CharactersPage, so a
// series' embedded cast and the cast returned by ListCharacters cannot drift
// apart.
func (s *Store) Characters(seriesID string, limit int) ([]*model.Character, error) {
	page, err := s.CharactersPage(seriesID, "", "", limit)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// SeriesTitle returns the titles of a series id, for naming a series a response
// references but does not embed.
func (s *Store) SeriesTitle(id string) (model.Title, bool) { return s.ix.SeriesTitle(id) }

// EpisodesPage returns one page of the episodes of a season or special.
//
// Reading the record parses the whole series, including every episode of every
// installment — that is inherent to one-record-per-series storage. What this
// bounds is the response: a caller asking for a 1000-episode season gets the
// page it asked for, not a megabyte of JSON.
func (s *Store) EpisodesPage(seasonID, specialID, token string, limit int) (Page[model.Episode], error) {
	id := seasonID
	if id == "" {
		id = specialID
	}
	seriesID, file, _, ok := s.ix.Work(id)
	if !ok {
		return Page[model.Episode]{}, nil
	}
	rec, err := s.record(file)
	if err != nil {
		return Page[model.Episode]{}, err
	}

	var episodes []model.Episode
	var found bool
	rec.EachSeries(func(series *model.Series) {
		if found || series.ID != seriesID {
			return
		}
		for i := range series.Seasons {
			if series.Seasons[i].ID == id && seasonID != "" {
				episodes, found = series.Seasons[i].Episodes, true
				return
			}
		}
		for i := range series.Specials {
			if series.Specials[i].ID == id && specialID != "" {
				episodes, found = series.Specials[i].Episodes, true
				return
			}
		}
	})
	if !found {
		// The id resolved to a work of the other kind — a movie, or a season
		// asked for as a special. Neither has the episodes requested.
		return Page[model.Episode]{}, nil
	}
	return index.Paginate(episodes, token, limit)
}

// SeriesPage returns one page of the series belonging to a franchise.
func (s *Store) SeriesPage(franchiseID, token string, limit int) (Page[*model.Series], error) {
	refs, err := s.ix.SeriesOf(franchiseID, token, limit)
	if err != nil {
		return Page[*model.Series]{}, err
	}
	out := Page[*model.Series]{NextToken: refs.NextToken, Total: refs.Total}
	for _, ref := range refs.Items {
		series, _, ok, err := s.Series(ref.ID)
		if err != nil {
			return Page[*model.Series]{}, err
		}
		if ok {
			out.Items = append(out.Items, series)
		}
	}
	return out, nil
}

// AppearancesPage returns one page of the series a character appears in.
func (s *Store) AppearancesPage(characterID, token string, limit int) (Page[model.CharacterAppearance], error) {
	c, ok, err := s.Character(characterID)
	if err != nil || !ok {
		return Page[model.CharacterAppearance]{}, err
	}
	return index.Paginate(c.Appearances, token, limit)
}

// CreditsPage returns one page of the roles a staff member is cast in.
func (s *Store) CreditsPage(staffID, token string, limit int) (Page[StaffCredit], error) {
	return s.ix.CreditsPage(staffID, token, limit)
}

// CharacterExists reports whether a character id is known, without reading its
// record.
func (s *Store) CharacterExists(id string) bool {
	_, ok := s.ix.Character(id)
	return ok
}

// WorkExists reports whether an installment id is known.
func (s *Store) WorkExists(id string) bool {
	_, _, _, ok := s.ix.Work(id)
	return ok
}

// FranchiseExists reports whether a franchise id is known, without reading its
// record.
func (s *Store) FranchiseExists(id string) (string, bool) { return s.ix.Franchise(id) }
