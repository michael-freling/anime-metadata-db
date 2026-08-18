package index

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// DefaultListLimit caps list results when the caller passes no limit.
const DefaultListLimit = 100

// DefaultSearchLimit caps search results when the caller passes no limit.
// Documented in web/content/docs/using-the-api.mdx and pinned by a test: a
// client that omits limit gets this many, so changing it changes a published
// contract rather than an implementation detail.
const DefaultSearchLimit = 50

// page walks n rows, keeps the ones match accepts, and materialises only those
// falling inside the requested window.
//
// One pass produces both the page and the total: the total is what tells a
// caller there is more to come, and computing it needs every row visited
// anyway. Nothing is copied out of the blob for the rows that are merely
// counted, which is what keeps a full scan cheap.
func page[T any](n int, match func(i int) bool, build func(i int) T, token string, limit int) (Page[T], error) {
	offset, err := decodeCursor(token)
	if err != nil {
		return Page[T]{}, err
	}
	if limit <= 0 {
		limit = DefaultListLimit
	}
	var out Page[T]
	end := offset + limit
	for i := 0; i < n; i++ {
		if !match(i) {
			continue
		}
		if out.Total >= offset && out.Total < end {
			out.Items = append(out.Items, build(i))
		}
		out.Total++
	}
	if end < out.Total {
		out.NextToken = encodeCursor(end)
	}
	return out, nil
}

// ErrInvalidPageToken is returned for a token this package did not issue.
//
// A sentinel rather than a message the caller greps: the service maps this to
// InvalidArgument, and deciding that by substring would silently reclassify any
// future error whose text happened to mention a page token.
var ErrInvalidPageToken = errors.New("invalid page token")

const cursorPrefix = "o:"

// encodeCursor renders an offset as an opaque page token.
func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorPrefix + strconv.Itoa(offset)))
}

// decodeCursor reads an offset back out of a page token. An empty token is the
// start of the result set; anything unparseable is rejected rather than
// silently treated as the start, so a corrupted token surfaces as an error
// instead of an unexpected first page.
//
// The token encodes a plain offset, which is sound because the index is
// immutable for the lifetime of a deployment — it is compiled into the binary,
// so there is no write path that could shift rows between two calls and make an
// offset skip or repeat an item. It is base64-encoded rather than sent as a
// bare integer so that clients cannot come to depend on the format.
func decodeCursor(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, ErrInvalidPageToken
	}
	s, ok := strings.CutPrefix(string(raw), cursorPrefix)
	if !ok {
		return 0, ErrInvalidPageToken
	}
	offset, err := strconv.Atoi(s)
	if err != nil || offset < 0 {
		return 0, ErrInvalidPageToken
	}
	return offset, nil
}

// Catalog returns one page of top-level entries, optionally restricted to a
// single kind, plus the total number matching. Entries keep the deterministic
// catalog order.
func (ix *Index) Catalog(kind *EntryKind, token string, limit int) (Page[CatalogEntry], error) {
	return page(len(ix.entries),
		func(i int) bool { return kind == nil || ix.entries[i].kind == *kind },
		ix.entryAt, token, limit)
}

// Search returns catalog entries whose original or translated title contains
// query (case-insensitive), one page at a time. A blank query matches nothing.
// Results keep the deterministic catalog order.
func (ix *Index) Search(query, token string, limit int) (Page[CatalogEntry], error) {
	q := NormalizeQuery(query)
	if q == "" {
		return Page[CatalogEntry]{}, nil
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	return page(len(ix.entries),
		func(i int) bool { return ix.entryTitlesMatch(i, q) },
		ix.entryAt, token, limit)
}

// Works returns one page of releases matching f, plus the total number
// matching. Works keep the deterministic catalog order.
func (ix *Index) Works(f WorkFilter, token string, limit int) (Page[Work], error) {
	q := NormalizeQuery(f.Query)

	// Resolve the series filter to a row once, rather than comparing an id
	// string per row. An unknown id matches nothing.
	var seriesRow int32 = -1
	if f.SeriesID != "" {
		row, ok := ix.seriesByID[f.SeriesID]
		if !ok {
			return Page[Work]{}, nil
		}
		seriesRow = row + 1
	}

	match := func(i int) bool {
		r := ix.works[i]
		switch {
		case f.ReleaseYear != 0 && int(r.year) != f.ReleaseYear:
			return false
		case f.ReleaseSeason != "" && r.quarter.text(ix.blob) != string(f.ReleaseSeason):
			return false
		case f.Kind != nil && r.kind != *f.Kind:
			return false
		case seriesRow >= 0 && r.series != seriesRow:
			return false
		}
		if q == "" {
			return true
		}
		return titleMatches(r.titles.text(ix.blob), q) ||
			titleMatches(ix.series[r.series-1].titles.text(ix.blob), q)
	}
	return page(len(ix.works), match, ix.workAt, token, limit)
}

// Characters returns one page of the cast of seriesID, or of the whole cast
// when seriesID is empty. query narrows by name, using the same
// case-insensitive substring match the catalog search applies to titles, so
// searching behaves the same wherever a name is involved.
//
// An unknown series id yields an empty page; the service layer is what turns a
// typo into NotFound.
func (ix *Index) Characters(seriesID, query, token string, limit int) (Page[Ref], error) {
	q := NormalizeQuery(query)

	var wantSeries int32 = -1
	if seriesID != "" {
		row, ok := ix.seriesByID[seriesID]
		if !ok {
			return Page[Ref]{}, nil
		}
		wantSeries = row + 1
	}

	match := func(i int) bool {
		r := ix.characters[i]
		if wantSeries >= 0 && !listHasRow(r.series.text(ix.blob), wantSeries) {
			return false
		}
		return q == "" || titleMatches(r.names.text(ix.blob), q)
	}
	build := func(i int) Ref {
		r := ix.characters[i]
		return Ref{ID: ix.text(r.id), File: ix.text(r.file)}
	}
	return page(len(ix.characters), match, build, token, limit)
}

// listHasRow reports whether a "|"-separated list column contains the given
// 1-based row number, without allocating.
func listHasRow(field string, want int32) bool {
	found := false
	eachElement(field, func(elem string) bool {
		n, err := strconv.Atoi(elem)
		if err == nil && int32(n) == want {
			found = true
			return false
		}
		return true
	})
	return found
}

// StaffPage returns staff, or only those credited in language when it is
// non-empty, one page at a time in deterministic dataset order.
func (ix *Index) StaffPage(language, query, token string, limit int) (Page[*model.Staff], error) {
	q := NormalizeQuery(query)
	match := func(i int) bool {
		r := ix.staff[i]
		if language != "" && !listHasString(r.langs.text(ix.blob), language) {
			return false
		}
		return q == "" || titleMatches(r.names.text(ix.blob), q)
	}
	return page(len(ix.staff), match, ix.staffAt, token, limit)
}

// listHasString reports whether a "|"-separated list column contains want.
func listHasString(field, want string) bool {
	found := false
	eachElement(field, func(elem string) bool {
		if unescape(elem) == want {
			found = true
			return false
		}
		return true
	})
	return found
}

// Franchise returns the record file holding the franchise with the given id.
func (ix *Index) Franchise(id string) (file string, ok bool) {
	row, ok := ix.franchiseByID[id]
	if !ok {
		return "", false
	}
	return ix.text(ix.franchises[row].file), true
}

// Franchises returns every franchise's id and record file, in catalog order.
func (ix *Index) Franchises() []Ref {
	out := make([]Ref, len(ix.franchises))
	for i, r := range ix.franchises {
		out[i] = Ref{ID: ix.text(r.id), File: ix.text(r.file)}
	}
	return out
}

// Series returns the record file holding the series with the given id, and the
// id of the franchise that owns it (empty for a standalone series).
func (ix *Index) Series(id string) (file, franchiseID string, ok bool) {
	row, ok := ix.seriesByID[id]
	if !ok {
		return "", "", false
	}
	r := ix.series[row]
	if r.franchise > 0 {
		franchiseID = ix.text(ix.franchises[r.franchise-1].id)
	}
	return ix.text(r.file), franchiseID, true
}

// Character returns the record file holding the character with the given id.
func (ix *Index) Character(id string) (file string, ok bool) {
	row, ok := ix.characterByID[id]
	if !ok {
		return "", false
	}
	return ix.text(ix.characters[row].file), true
}

// Staff returns the staff member with the given id. Staff records are small
// enough to live in the index in full, so this never reads a file.
func (ix *Index) Staff(id string) (*model.Staff, bool) {
	row, ok := ix.staffByID[id]
	if !ok {
		return nil, false
	}
	return ix.staffAt(int(row)), true
}

// Credits returns every role the staff member is cast in, in deterministic
// dataset order (nil for an unknown or uncredited id).
//
// A credit names a character by id and name, both of which the index carries,
// so a staff page renders without opening a single record file.
func (ix *Index) Credits(staffID string) []StaffCredit {
	row, ok := ix.staffByID[staffID]
	if !ok {
		return nil
	}
	r, ok := ix.creditsByStaff[row+1]
	if !ok {
		return nil
	}
	out := make([]StaffCredit, 0, r[1]-r[0])
	for i := r[0]; i < r[1]; i++ {
		cr := ix.credits[i]
		if cr.staff != row+1 {
			continue
		}
		c := ix.characters[cr.character-1]
		credit := StaffCredit{
			CharacterID:    ix.text(c.id),
			CharacterNames: unpackTitle(c.names.text(ix.blob)),
			Language:       ix.text(cr.language),
		}
		eachElement(cr.series.text(ix.blob), func(elem string) bool {
			n, err := strconv.Atoi(elem)
			if err == nil && n >= 1 && n <= len(ix.series) {
				credit.SeriesIDs = append(credit.SeriesIDs, ix.text(ix.series[n-1].id))
			}
			return true
		})
		out = append(out, credit)
	}
	return out
}

// SeriesTitle returns the resolved-at-call-site titles of a series id, for
// naming a series a response references but does not embed.
func (ix *Index) SeriesTitle(id string) (model.Title, bool) {
	row, ok := ix.seriesByID[id]
	if !ok {
		return model.Title{}, false
	}
	return unpackTitle(ix.series[row].titles.text(ix.blob)), true
}

// Paginate slices a caller's own slice with the same cursor scheme the index
// uses, so a listing assembled outside this package pages identically.
func Paginate[T any](in []T, token string, limit int) (Page[T], error) {
	return page(len(in), func(int) bool { return true }, func(i int) T { return in[i] }, token, limit)
}

// EmbeddedLimit bounds every collection a Get* response embeds.
//
// A Get response is a convenience — one call that shows you the shape of a
// node — not a bulk export. Left unbounded, GetFranchise would serialize every
// episode of every season of every series under a brand, which is the exact
// cost this package exists to avoid. The matching *_total field carries the
// real count, and a List RPC pages the rest.
const EmbeddedLimit = 25

// Work resolves an installment id — a season, movie or special — to the series
// it belongs to and the record file holding it.
func (ix *Index) Work(id string) (seriesID, file string, kind WorkKind, ok bool) {
	row, ok := ix.workByID[id]
	if !ok {
		return "", "", 0, false
	}
	w := ix.works[row]
	s := ix.series[w.series-1]
	return ix.text(s.id), ix.text(s.file), w.kind, true
}

// SeriesOf returns one page of the series belonging to franchiseID, in catalog
// order. An unknown franchise yields an empty page.
func (ix *Index) SeriesOf(franchiseID, token string, limit int) (Page[Ref], error) {
	row, ok := ix.franchiseByID[franchiseID]
	if !ok {
		return Page[Ref]{}, nil
	}
	want := row + 1
	match := func(i int) bool { return ix.series[i].franchise == want }
	build := func(i int) Ref {
		r := ix.series[i]
		return Ref{ID: ix.text(r.id), File: ix.text(r.file)}
	}
	return page(len(ix.series), match, build, token, limit)
}

// CreditsPage returns one page of the roles staffID is cast in.
//
// Credits are materialised from the index alone — a credit names a character by
// id and name, both of which the index carries — so paging them reads no file.
func (ix *Index) CreditsPage(staffID, token string, limit int) (Page[StaffCredit], error) {
	return Paginate(ix.Credits(staffID), token, limit)
}
