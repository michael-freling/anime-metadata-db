package index

import (
	"fmt"

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

// The single-letter codes the index file uses for a work kind. They are written
// out rather than numbered so a row is readable without the schema.
const (
	workSeasonCode  = "season"
	workMovieCode   = "movie"
	workSpecialCode = "special"
)

func workKindCode(k WorkKind) string {
	switch k {
	case WorkMovie:
		return workMovieCode
	case WorkSpecial:
		return workSpecialCode
	default:
		return workSeasonCode
	}
}

func parseWorkKind(code string) (WorkKind, error) {
	switch code {
	case workSeasonCode:
		return WorkSeason, nil
	case workMovieCode:
		return WorkMovie, nil
	case workSpecialCode:
		return WorkSpecial, nil
	default:
		return 0, fmt.Errorf("unknown work kind %q", code)
	}
}

// EntryKind classifies a catalog entry.
type EntryKind int

// The catalog entry kinds.
const (
	EntryFranchise EntryKind = iota
	EntrySeries
)

// The codes the index file uses for an entry kind.
const (
	entryFranchise = "franchise"
	entrySeries    = "series"
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

// CatalogEntry is a searchable top-level node: a franchise or a standalone
// series. FranchiseID is set only for a series owned by a franchise.
type CatalogEntry struct {
	Kind        EntryKind
	ID          string
	Titles      model.Title
	FranchiseID string

	// Aggregates over everything beneath the entry, so listing the catalog
	// never walks the hierarchy. The year span is 0/0 when nothing beneath it
	// carries a release year.
	FirstReleaseYear  int
	LatestReleaseYear int
	Works             int
	Episodes          int
}

// Stats summarizes the dataset.
type Stats struct {
	Franchises int
	Series     int
	Seasons    int
	Episodes   int
	Characters int
	Staff      int

	// The span of release years the dataset actually covers. Both are 0 when
	// nothing in it carries a year. EarliestReleaseYear is the floor below which
	// a year cannot be real data for this dataset.
	EarliestReleaseYear int
	LatestReleaseYear   int
}

// StaffCredit is one role a staff member is cast in: the character they voice,
// the language, and the series the casting applies to.
//
// It carries the character's id and names rather than the character record: a
// credit only ever renders those two, and holding them here is what lets a
// staff page render without reopening any data file.
type StaffCredit struct {
	CharacterID    string
	CharacterNames model.Title
	Language       string
	SeriesIDs      []string
}

// Ref names a record and the data file that holds it. Listings hand these back
// so a caller can materialise only the page it is returning.
type Ref struct {
	ID   string
	File string
}

// WorkFilter narrows a work listing. A zero field matches everything, so the
// zero filter walks the entire dataset.
type WorkFilter struct {
	ReleaseYear   int
	ReleaseSeason model.ReleaseSeason
	Kind          *WorkKind
	SeriesID      string
	// Query matches the work's own title or its series' title. A work often has
	// no title of its own — an untitled season is just "Season 2" — so matching
	// the series too is what makes searching releases useful.
	Query string
}

// Page is one slice of a longer result set.
type Page[T any] struct {
	Items []T
	// NextToken is empty when Items reaches the end of the result set.
	NextToken string
	// Total counts every match, not just this page.
	Total int
}
