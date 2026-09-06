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

	"github.com/michael-freling/anime-metadata-db/api/internal/index"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// The types the service works in are the index's: listing a browse page and
// reading the index are the same operation, so there is nothing for this
// package to redefine.
type (
	// CatalogEntry is a searchable top-level node.
	CatalogEntry = index.CatalogEntry
	// SeriesEntry is one series as a search result.
	SeriesEntry = index.SeriesEntry
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

// SeriesSearch returns one page of series matching query, with the aggregates a
// result row needs. It reads no record files: everything a SeriesEntry carries
// is already in the index.
func (s *Store) SeriesSearch(query, token string, limit int) (Page[SeriesEntry], error) {
	return s.ix.SeriesSearch(query, token, limit)
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

// SeriesCast returns the entire cast of seriesID, in dataset order. GetSeries
// embeds a series whole, so this is unpaged by design; the bound is the size of
// one series' cast, which is a few hundred names at most.
func (s *Store) SeriesCast(seriesID string) ([]*model.Character, error) {
	refs := s.ix.CharacterRefs(seriesID)
	out := make([]*model.Character, 0, len(refs))
	for _, ref := range refs {
		c, err := s.characterIn(ref.File, ref.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
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

// Work resolves an installment id to its indexed row, for labelling a scope
// without reopening the record it lives in.
func (s *Store) Work(id string) (index.Work, bool) { return s.ix.WorkByID(id) }

// WorkExists reports whether an installment id is known.
func (s *Store) WorkExists(id string) bool {
	_, _, _, ok := s.ix.Work(id)
	return ok
}

// FranchiseExists reports whether a franchise id is known, without reading its
// record.
func (s *Store) FranchiseExists(id string) (string, bool) { return s.ix.Franchise(id) }
