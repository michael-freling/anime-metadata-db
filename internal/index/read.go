package index

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// Index is a read-only view over an index file. Opening one records where each
// row starts; no field is copied out of the blob until a request materialises
// the rows it is returning, so the resident cost is the row tables — a few
// bytes per record — and not the data itself.
//
// It is immutable once opened and safe for concurrent use.
type Index struct {
	blob string

	stats      Stats
	franchises []franchiseRow
	series     []seriesRow
	entries    []entryRow
	works      []workRow
	characters []characterRow
	staff      []staffRow
	credits    []creditRow

	// Id lookups. These are the only maps the index holds; everything else is
	// a slice indexed by row number.
	franchiseByID map[string]int32
	seriesByID    map[string]int32
	characterByID map[string]int32
	staffByID     map[string]int32
	// workByID resolves a season, movie or special id to its works row, so a
	// request naming one installment can find the record file holding it
	// without scanning.
	workByID map[string]int32

	// creditsByStaff[row] is the half-open range of credits belonging to staff
	// row. Credits are written grouped by staff, so a range replaces a map of
	// slices.
	creditsByStaff map[int32][2]int32
}

type franchiseRow struct{ id, file, titles span }

type seriesRow struct {
	id, file, titles span
	franchise        int32 // 1-based; 0 for a standalone series
}

type entryRow struct {
	kind                           EntryKind
	row                            int32 // 1-based, into franchises or series
	first, latest, works, episodes int32
}

type workRow struct {
	kind                       WorkKind
	id, titles                 span
	date, quarter, format      span
	wikidata                   span
	series                     int32 // 1-based
	number, year, episodes     int32
	anilist, anidb, tmdb, tvdb int32
}

type characterRow struct{ id, file, names, series span }

type staffRow struct {
	id, file, names, langs     span
	wikidata                   span
	anilist, anidb, tmdb, tvdb int32
}

type creditRow struct {
	staff, character int32 // 1-based
	language, series span
}

// Open parses the index file in blob. blob is retained, not copied: it is meant
// to be an embedded string, which lives in the binary's read-only data.
func Open(blob string) (*Index, error) {
	ix := &Index{
		blob:           blob,
		franchiseByID:  map[string]int32{},
		seriesByID:     map[string]int32{},
		characterByID:  map[string]int32{},
		staffByID:      map[string]int32{},
		workByID:       map[string]int32{},
		creditsByStaff: map[int32][2]int32{},
	}
	if err := ix.scan(); err != nil {
		return nil, err
	}
	ix.indexCredits()
	return ix, nil
}

// scan walks the blob once, dispatching each row to its section.
func (ix *Index) scan() error {
	rest := ix.blob
	var offset uint32
	section := ""
	line := 0

	first := true
	for len(rest) > 0 {
		nl := strings.IndexByte(rest, '\n')
		var row string
		if nl < 0 {
			row, rest = rest, ""
		} else {
			row, rest = rest[:nl], rest[nl+1:]
		}
		start := offset
		offset += uint32(len(row)) + 1
		line++

		if row == "" {
			continue
		}
		if first {
			first = false
			if err := checkHeader(row); err != nil {
				return err
			}
			continue
		}
		if row[0] == '!' {
			name, params, _ := strings.Cut(row[1:], "\t")
			section = name
			if name == sectionStats {
				if err := ix.readStats(params); err != nil {
					return errRow(sectionStats, line, err)
				}
			}
			// Any other section line is the column header, which the reader
			// does not need — it is there for whoever reads the file.
			continue
		}
		if err := ix.readRow(section, row, start); err != nil {
			return errRow(section, line, err)
		}
	}
	if first {
		return fmt.Errorf("index: empty")
	}
	return nil
}

// checkHeader validates the magic and version line.
func checkHeader(row string) error {
	name, version, ok := strings.Cut(row, "\t")
	if !ok || name != magic {
		return fmt.Errorf("index: not an index file")
	}
	v, err := strconv.Atoi(version)
	if err != nil {
		return fmt.Errorf("index: unreadable version %q", version)
	}
	if v != Version {
		return fmt.Errorf("index: version %d, want %d — regenerate with `make index`", v, Version)
	}
	return nil
}

// readStats parses the key=value pairs of the stats line.
func (ix *Index) readStats(params string) error {
	for _, pair := range strings.Split(params, "\t") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return fmt.Errorf("malformed stat %q", pair)
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("stat %s: %w", key, err)
		}
		switch key {
		case "franchises":
			ix.stats.Franchises = n
		case "series":
			ix.stats.Series = n
		case "seasons":
			ix.stats.Seasons = n
		case "episodes":
			ix.stats.Episodes = n
		case "characters":
			ix.stats.Characters = n
		case "staff":
			ix.stats.Staff = n
		case "earliest":
			ix.stats.EarliestReleaseYear = n
		case "latest":
			ix.stats.LatestReleaseYear = n
		}
	}
	return nil
}

// cols splits a row into exactly n columns, as spans relative to the blob.
func cols(row string, start uint32, n int) ([]span, error) {
	out := make([]span, 0, n)
	var off uint32
	for {
		i := strings.IndexByte(row, '\t')
		if i < 0 {
			out = append(out, span{start + off, uint32(len(row))})
			break
		}
		out = append(out, span{start + off, uint32(i)})
		off += uint32(i) + 1
		row = row[i+1:]
	}
	if len(out) != n {
		return nil, fmt.Errorf("got %d columns, want %d", len(out), n)
	}
	return out, nil
}

// readRow dispatches one data row to its section's parser.
func (ix *Index) readRow(section, row string, start uint32) error {
	switch section {
	case sectionFranchises:
		return ix.readFranchise(row, start)
	case sectionSeries:
		return ix.readSeries(row, start)
	case sectionEntries:
		return ix.readEntry(row, start)
	case sectionWorks:
		return ix.readWork(row, start)
	case sectionCharacters:
		return ix.readCharacter(row, start)
	case sectionStaff:
		return ix.readStaff(row, start)
	case sectionCredits:
		return ix.readCredit(row, start)
	case "":
		return fmt.Errorf("row before any section")
	default:
		// An unknown section is a newer writer adding data this reader does not
		// use. Skipping it keeps a rolling deploy — old server, new index —
		// serving rather than refusing to boot.
		return nil
	}
}

func (ix *Index) readFranchise(row string, start uint32) error {
	c, err := cols(row, start, 3)
	if err != nil {
		return err
	}
	ix.franchiseByID[unescape(c[0].text(ix.blob))] = int32(len(ix.franchises))
	ix.franchises = append(ix.franchises, franchiseRow{id: c[0], file: c[1], titles: c[2]})
	return nil
}

func (ix *Index) readSeries(row string, start uint32) error {
	c, err := cols(row, start, 4)
	if err != nil {
		return err
	}
	franchise, err := parseInt(c[2].text(ix.blob))
	if err != nil {
		return fmt.Errorf("franchise: %w", err)
	}
	if franchise > len(ix.franchises) {
		return fmt.Errorf("franchise row %d out of range", franchise)
	}
	ix.seriesByID[unescape(c[0].text(ix.blob))] = int32(len(ix.series))
	ix.series = append(ix.series, seriesRow{
		id: c[0], file: c[1], franchise: int32(franchise), titles: c[3],
	})
	return nil
}

func (ix *Index) readEntry(row string, start uint32) error {
	c, err := cols(row, start, 6)
	if err != nil {
		return err
	}
	var e entryRow
	switch c[0].text(ix.blob) {
	case entryFranchise:
		e.kind = EntryFranchise
	case entrySeries:
		e.kind = EntrySeries
	default:
		return fmt.Errorf("unknown entry kind %q", c[0].text(ix.blob))
	}
	nums := make([]int, 5)
	for i := range nums {
		if nums[i], err = parseInt(c[i+1].text(ix.blob)); err != nil {
			return err
		}
	}
	e.row = int32(nums[0])
	limit := len(ix.series)
	if e.kind == EntryFranchise {
		limit = len(ix.franchises)
	}
	if nums[0] < 1 || nums[0] > limit {
		return fmt.Errorf("row %d out of range", nums[0])
	}
	e.first, e.latest = int32(nums[1]), int32(nums[2])
	e.works, e.episodes = int32(nums[3]), int32(nums[4])
	ix.entries = append(ix.entries, e)
	return nil
}

func (ix *Index) readWork(row string, start uint32) error {
	c, err := cols(row, start, 15)
	if err != nil {
		return err
	}
	kind, err := parseWorkKind(c[0].text(ix.blob))
	if err != nil {
		return err
	}
	nums := map[int]int{}
	for _, i := range []int{2, 3, 5, 8, 10, 11, 12, 13} {
		if nums[i], err = parseInt(c[i].text(ix.blob)); err != nil {
			return err
		}
	}
	if nums[2] < 1 || nums[2] > len(ix.series) {
		return fmt.Errorf("series row %d out of range", nums[2])
	}
	ix.workByID[unescape(c[1].text(ix.blob))] = int32(len(ix.works))
	ix.works = append(ix.works, workRow{
		kind: kind, id: c[1], series: int32(nums[2]), number: int32(nums[3]),
		date: c[4], year: int32(nums[5]), quarter: c[6], format: c[7],
		episodes: int32(nums[8]), titles: c[9],
		anilist: int32(nums[10]), anidb: int32(nums[11]),
		tmdb: int32(nums[12]), tvdb: int32(nums[13]), wikidata: c[14],
	})
	return nil
}

func (ix *Index) readCharacter(row string, start uint32) error {
	c, err := cols(row, start, 4)
	if err != nil {
		return err
	}
	ix.characterByID[unescape(c[0].text(ix.blob))] = int32(len(ix.characters))
	ix.characters = append(ix.characters, characterRow{
		id: c[0], file: c[1], names: c[2], series: c[3],
	})
	return nil
}

func (ix *Index) readStaff(row string, start uint32) error {
	c, err := cols(row, start, 9)
	if err != nil {
		return err
	}
	nums := make([]int, 4)
	for i := range nums {
		if nums[i], err = parseInt(c[i+4].text(ix.blob)); err != nil {
			return err
		}
	}
	ix.staffByID[unescape(c[0].text(ix.blob))] = int32(len(ix.staff))
	ix.staff = append(ix.staff, staffRow{
		id: c[0], file: c[1], names: c[2], langs: c[3],
		anilist: int32(nums[0]), anidb: int32(nums[1]),
		tmdb: int32(nums[2]), tvdb: int32(nums[3]), wikidata: c[8],
	})
	return nil
}

func (ix *Index) readCredit(row string, start uint32) error {
	c, err := cols(row, start, 4)
	if err != nil {
		return err
	}
	staff, err := parseInt(c[0].text(ix.blob))
	if err != nil {
		return err
	}
	character, err := parseInt(c[1].text(ix.blob))
	if err != nil {
		return err
	}
	if staff < 1 || staff > len(ix.staff) {
		return fmt.Errorf("staff row %d out of range", staff)
	}
	if character < 1 || character > len(ix.characters) {
		return fmt.Errorf("character row %d out of range", character)
	}
	ix.credits = append(ix.credits, creditRow{
		staff: int32(staff), character: int32(character), language: c[2], series: c[3],
	})
	return nil
}

// indexCredits records the contiguous range of credits belonging to each staff
// member. The writer groups them, so a range suffices; a row that breaks the
// grouping would only widen the range, never lose a credit, because the range
// is extended to cover every row seen for that staff member.
func (ix *Index) indexCredits() {
	for i, cr := range ix.credits {
		r, ok := ix.creditsByStaff[cr.staff]
		if !ok {
			ix.creditsByStaff[cr.staff] = [2]int32{int32(i), int32(i) + 1}
			continue
		}
		r[1] = int32(i) + 1
		ix.creditsByStaff[cr.staff] = r
	}
}

// Stats returns the dataset summary recorded in the index.
func (ix *Index) Stats() Stats { return ix.stats }

// --- Materialising rows ----------------------------------------------------
//
// Everything below turns rows back into ordinary Go values. These allocate, so
// they run only for the rows a response carries, never inside a filter.

func (ix *Index) text(s span) string { return unescape(s.text(ix.blob)) }

// seriesTitles returns the titles of a 1-based series row.
func (ix *Index) seriesTitles(row int32) model.Title {
	return unpackTitle(ix.series[row-1].titles.text(ix.blob))
}

// workAt materialises the work at row i.
func (ix *Index) workAt(i int) Work {
	r := ix.works[i]
	w := Work{
		Kind:          r.kind,
		ID:            ix.text(r.id),
		Titles:        unpackTitle(r.titles.text(ix.blob)),
		SeriesID:      ix.text(ix.series[r.series-1].id),
		SeriesTitles:  ix.seriesTitles(r.series),
		Number:        int(r.number),
		ReleaseYear:   int(r.year),
		ReleaseSeason: model.ReleaseSeason(ix.text(r.quarter)),
		Format:        model.SpecialFormat(ix.text(r.format)),
		EpisodeCount:  int(r.episodes),
		ExternalIDs: model.ExternalIDs{
			AnilistID: int(r.anilist), AnidbID: int(r.anidb),
			TmdbID: int(r.tmdb), TvdbID: int(r.tvdb),
			WikidataID: ix.text(r.wikidata),
		},
	}
	if d := r.date.text(ix.blob); d != "" {
		if parsed, err := model.ParseDate(d); err == nil {
			w.ReleaseDate = parsed
		}
	}
	return w
}

// entryAt materialises the catalog entry at row i.
func (ix *Index) entryAt(i int) CatalogEntry {
	r := ix.entries[i]
	e := CatalogEntry{
		Kind:              r.kind,
		FirstReleaseYear:  int(r.first),
		LatestReleaseYear: int(r.latest),
		Works:             int(r.works),
		Episodes:          int(r.episodes),
	}
	if r.kind == EntryFranchise {
		f := ix.franchises[r.row-1]
		e.ID = ix.text(f.id)
		e.Titles = unpackTitle(f.titles.text(ix.blob))
		return e
	}
	s := ix.series[r.row-1]
	e.ID = ix.text(s.id)
	e.Titles = unpackTitle(s.titles.text(ix.blob))
	if s.franchise > 0 {
		e.FranchiseID = ix.text(ix.franchises[s.franchise-1].id)
	}
	return e
}

// staffAt materialises the staff member at row i.
func (ix *Index) staffAt(i int) *model.Staff {
	r := ix.staff[i]
	return &model.Staff{
		ID:    ix.text(r.id),
		Names: unpackTitle(r.names.text(ix.blob)),
		ExternalIDs: model.ExternalIDs{
			AnilistID: int(r.anilist), AnidbID: int(r.anidb),
			TmdbID: int(r.tmdb), TvdbID: int(r.tvdb),
			WikidataID: ix.text(r.wikidata),
		},
	}
}

// entryTitlesMatch reports whether the catalog entry at row i matches the
// already-lowercased query, without materialising it.
func (ix *Index) entryTitlesMatch(i int, query string) bool {
	r := ix.entries[i]
	if r.kind == EntryFranchise {
		return titleMatches(ix.franchises[r.row-1].titles.text(ix.blob), query)
	}
	return titleMatches(ix.series[r.row-1].titles.text(ix.blob), query)
}
