package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	animev1 "github.com/michael-freling/anime-metadata-db/internal/gen/anime/v1"
	"github.com/michael-freling/anime-metadata-db/internal/gen/anime/v1/animev1connect"
)

// Service implements the anime.v1.AnimeService Connect handler over a Store.
type Service struct {
	store   *Store
	version string
}

// compile-time assertion that Service satisfies the generated handler.
var _ animev1connect.AnimeServiceHandler = (*Service)(nil)

// NewService returns a Service backed by store. version is reported by
// GetHealth.
func NewService(store *Store, version string) *Service {
	return &Service{store: store, version: version}
}

// ListFranchises returns every franchise in the catalog.
func (s *Service) ListFranchises(_ context.Context, req *connect.Request[animev1.ListFranchisesRequest]) (*connect.Response[animev1.ListFranchisesResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	franchises := s.store.Franchises()
	out := make([]*animev1.Franchise, len(franchises))
	for i, f := range franchises {
		out[i] = toFranchise(loc, s.store, f)
	}
	return connect.NewResponse(&animev1.ListFranchisesResponse{Franchises: out}), nil
}

// GetFranchise returns one franchise by id, or CodeNotFound.
func (s *Service) GetFranchise(_ context.Context, req *connect.Request[animev1.GetFranchiseRequest]) (*connect.Response[animev1.GetFranchiseResponse], error) {
	id := req.Msg.GetId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	f, ok := s.store.Franchise(id)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("franchise %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	return connect.NewResponse(&animev1.GetFranchiseResponse{Franchise: toFranchise(loc, s.store, f)}), nil
}

// GetSeries returns one series by id (under a franchise or standalone), or
// CodeNotFound.
func (s *Service) GetSeries(_ context.Context, req *connect.Request[animev1.GetSeriesRequest]) (*connect.Response[animev1.GetSeriesResponse], error) {
	id := req.Msg.GetId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	series, franchiseID, ok := s.store.Series(id)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("series %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	return connect.NewResponse(&animev1.GetSeriesResponse{
		Series:      toSeries(loc, s.store, series),
		FranchiseId: franchiseID,
	}), nil
}

// Search matches franchises and series by title.
func (s *Service) Search(_ context.Context, req *connect.Request[animev1.SearchRequest]) (*connect.Response[animev1.SearchResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.SearchPage(req.Msg.GetQuery(), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	out := make([]*animev1.SearchResult, len(page.Items))
	for i, m := range page.Items {
		out[i] = toSearchResult(loc, m)
	}
	return connect.NewResponse(&animev1.SearchResponse{
		Results:       out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// ListCatalog pages through the top-level entries as flat summaries. Unlike
// ListFranchises it nests nothing, so a page stays the same size however large
// the catalog grows.
func (s *Service) ListCatalog(_ context.Context, req *connect.Request[animev1.ListCatalogRequest]) (*connect.Response[animev1.ListCatalogResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.Catalog(fromEntryKind(req.Msg.GetKind()), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	out := make([]*animev1.CatalogEntry, len(page.Items))
	for i, e := range page.Items {
		out[i] = toCatalogEntry(loc, e)
	}
	return connect.NewResponse(&animev1.ListCatalogResponse{
		Entries:       out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// ListWorks pages through individual releases, filtered by year, quarter, kind
// or series. An unknown series_id is CodeNotFound rather than an empty page, so
// a typo is not mistaken for a series with no releases.
func (s *Service) ListWorks(_ context.Context, req *connect.Request[animev1.ListWorksRequest]) (*connect.Response[animev1.ListWorksResponse], error) {
	seriesID := req.Msg.GetSeriesId()
	if seriesID != "" {
		if _, _, ok := s.store.Series(seriesID); !ok {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("series %q not found", seriesID))
		}
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	filter := WorkFilter{
		ReleaseYear:   int(req.Msg.GetReleaseYear()),
		ReleaseSeason: fromReleaseSeason(req.Msg.GetReleaseSeason()),
		Kind:          fromWorkKind(req.Msg.GetKind()),
		SeriesID:      seriesID,
	}
	page, err := s.store.Works(filter, req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	out := make([]*animev1.WorkSummary, len(page.Items))
	for i, w := range page.Items {
		out[i] = toWorkSummary(loc, w)
	}
	return connect.NewResponse(&animev1.ListWorksResponse{
		Works:         out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// GetCharacter returns one character by id, or CodeNotFound.
func (s *Service) GetCharacter(_ context.Context, req *connect.Request[animev1.GetCharacterRequest]) (*connect.Response[animev1.GetCharacterResponse], error) {
	id := req.Msg.GetId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	c, ok := s.store.Character(id)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("character %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	return connect.NewResponse(&animev1.GetCharacterResponse{Character: toCharacter(loc, s.store, c)}), nil
}

// ListCharacters returns the whole cast, or one series' cast when series_id is
// set. An unknown series_id is CodeNotFound rather than an empty list, so a
// typo is not mistaken for a series with no cast.
func (s *Service) ListCharacters(_ context.Context, req *connect.Request[animev1.ListCharactersRequest]) (*connect.Response[animev1.ListCharactersResponse], error) {
	seriesID := req.Msg.GetSeriesId()
	if seriesID != "" {
		if _, _, ok := s.store.Series(seriesID); !ok {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("series %q not found", seriesID))
		}
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.CharactersPage(seriesID, req.Msg.GetQuery(), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&animev1.ListCharactersResponse{
		Characters:    toCharacters(loc, s.store, page.Items),
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// GetStaff returns one staff member by id with their credits, or CodeNotFound.
func (s *Service) GetStaff(_ context.Context, req *connect.Request[animev1.GetStaffRequest]) (*connect.Response[animev1.GetStaffResponse], error) {
	id := req.Msg.GetId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	st, ok := s.store.Staff(id)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("staff %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	return connect.NewResponse(&animev1.GetStaffResponse{
		Staff:   toStaff(loc, st),
		Credits: toStaffCredits(loc, s.store.StaffCredits(id)),
	}), nil
}

// ListStaff returns every staff member, optionally filtered to those credited
// in one language.
func (s *Service) ListStaff(_ context.Context, req *connect.Request[animev1.ListStaffRequest]) (*connect.Response[animev1.ListStaffResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.StaffPage(req.Msg.GetLanguage(), req.Msg.GetQuery(), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&animev1.ListStaffResponse{
		Staff:         toStaffList(loc, page.Items),
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// GetHealth reports liveness, build version and dataset stats.
func (s *Service) GetHealth(_ context.Context, _ *connect.Request[animev1.GetHealthRequest]) (*connect.Response[animev1.GetHealthResponse], error) {
	st := s.store.Stats()
	return connect.NewResponse(&animev1.GetHealthResponse{
		Status:  "ok",
		Version: s.version,
		Stats: &animev1.DatasetStats{
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
