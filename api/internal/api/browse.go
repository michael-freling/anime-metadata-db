package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	browsev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/browse/v1"
	"github.com/michael-freling/anime-metadata-db/api/internal/gen/browse/v1/browsev1connect"
)

// BrowseService implements browse.v1.BrowseService: the undocumented API behind
// anime-metadata-web.
//
// It reads the same Store as the public Service and reuses its converters, so
// the two cannot disagree about what a series or a character looks like. What
// it adds is the shapes only a browse UI wants — flat catalog rows with
// precomputed counts, the franchise grouping, the global character and staff
// indexes — kept here so the public API does not have to carry them.
//
// Anything the public API already answers (searching series, searching
// releases, opening one series) is absent here on purpose: the site calls
// anime.v1 for those.
type BrowseService struct {
	store   *Store
	version string
}

// compile-time assertion that BrowseService satisfies the generated handler.
var _ browsev1connect.BrowseServiceHandler = (*BrowseService)(nil)

// NewBrowseService returns a BrowseService backed by store. version is reported
// by GetStats.
func NewBrowseService(store *Store, version string) *BrowseService {
	return &BrowseService{store: store, version: version}
}

// Search matches franchises and standalone series by title.
func (s *BrowseService) Search(_ context.Context, req *connect.Request[browsev1.SearchRequest]) (*connect.Response[browsev1.SearchResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.SearchPage(req.Msg.GetQuery(), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]*browsev1.SearchResult, len(page.Items))
	for i, m := range page.Items {
		out[i] = toSearchResult(loc, m)
	}
	return connect.NewResponse(&browsev1.SearchResponse{
		Results:       out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// ListCatalog pages through the top-level entries as flat summary rows. It
// nests nothing, so a page stays the same size however large the catalog grows.
func (s *BrowseService) ListCatalog(_ context.Context, req *connect.Request[browsev1.ListCatalogRequest]) (*connect.Response[browsev1.ListCatalogResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.Catalog(fromEntryKind(req.Msg.GetKind()), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]*browsev1.CatalogEntry, len(page.Items))
	for i, e := range page.Items {
		out[i] = toCatalogEntry(loc, e)
	}
	return connect.NewResponse(&browsev1.ListCatalogResponse{
		Entries:       out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// GetFranchise returns one franchise by id with each of its series embedded
// whole, or CodeNotFound.
func (s *BrowseService) GetFranchise(_ context.Context, req *connect.Request[browsev1.GetFranchiseRequest]) (*connect.Response[browsev1.GetFranchiseResponse], error) {
	id := req.Msg.GetId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	f, ok, err := s.store.Franchise(id)
	if err != nil {
		return nil, storeError(err)
	}
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("franchise %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	out, err := toFranchise(loc, s.store, f)
	if err != nil {
		return nil, storeError(err)
	}
	return connect.NewResponse(&browsev1.GetFranchiseResponse{Franchise: out}), nil
}

// GetCharacter returns one character by id with every appearance, or
// CodeNotFound.
func (s *BrowseService) GetCharacter(_ context.Context, req *connect.Request[browsev1.GetCharacterRequest]) (*connect.Response[browsev1.GetCharacterResponse], error) {
	id := req.Msg.GetId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	c, ok, err := s.store.Character(id)
	if err != nil {
		return nil, storeError(err)
	}
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("character %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	return connect.NewResponse(&browsev1.GetCharacterResponse{Character: toCharacter(loc, s.store, "", c)}), nil
}

// ListCharacters returns the whole cast, or one series' cast when series_id is
// set. An unknown series_id is CodeNotFound rather than an empty list, so a
// typo is not mistaken for a series with no cast.
func (s *BrowseService) ListCharacters(_ context.Context, req *connect.Request[browsev1.ListCharactersRequest]) (*connect.Response[browsev1.ListCharactersResponse], error) {
	seriesID := req.Msg.GetSeriesId()
	if seriesID != "" {
		if !s.store.SeriesExists(seriesID) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("series %q not found", seriesID))
		}
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.CharactersPage(seriesID, req.Msg.GetQuery(), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	return connect.NewResponse(&browsev1.ListCharactersResponse{
		Characters:    toCharacters(loc, s.store, seriesID, page.Items),
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// GetStaff returns one staff member by id with every role they are cast in, or
// CodeNotFound. Credits are embedded whole for the same reason a series' cast
// is: a career is bounded, and a truncated list is worse than a long one.
func (s *BrowseService) GetStaff(_ context.Context, req *connect.Request[browsev1.GetStaffRequest]) (*connect.Response[browsev1.GetStaffResponse], error) {
	id := req.Msg.GetId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	st, ok := s.store.Staff(id)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("staff %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	return connect.NewResponse(&browsev1.GetStaffResponse{
		Staff:   toStaff(loc, st),
		Credits: toStaffCredits(loc, s.store, s.store.StaffCredits(id)),
	}), nil
}

// ListStaff returns every staff member, optionally filtered to those credited
// in one language.
func (s *BrowseService) ListStaff(_ context.Context, req *connect.Request[browsev1.ListStaffRequest]) (*connect.Response[browsev1.ListStaffResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.StaffPage(req.Msg.GetLanguage(), req.Msg.GetQuery(), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	return connect.NewResponse(&browsev1.ListStaffResponse{
		Staff:         toStaffList(loc, page.Items),
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// GetStats reports what the dataset contains, plus the deployed revision.
func (s *BrowseService) GetStats(_ context.Context, _ *connect.Request[browsev1.GetStatsRequest]) (*connect.Response[browsev1.GetStatsResponse], error) {
	st := s.store.Stats()
	return connect.NewResponse(&browsev1.GetStatsResponse{
		Version: s.version,
		Stats: &browsev1.DatasetStats{
			Franchises: int32(st.Franchises),
			Series:     int32(st.Series),
			Seasons:    int32(st.Seasons),
			Episodes:   int32(st.Episodes),
			Characters: int32(st.Characters),
			Staff:      int32(st.Staff),

			EarliestReleaseYear: int32(st.EarliestReleaseYear),
			LatestReleaseYear:   int32(st.LatestReleaseYear),
		},
	}), nil
}
