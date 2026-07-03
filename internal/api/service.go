package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	animev1 "github.com/michael-freling/anime-metadata-db/gen/anime/v1"
	"github.com/michael-freling/anime-metadata-db/gen/anime/v1/animev1connect"
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
func (s *Service) ListFranchises(_ context.Context, _ *connect.Request[animev1.ListFranchisesRequest]) (*connect.Response[animev1.ListFranchisesResponse], error) {
	franchises := s.store.Franchises()
	out := make([]*animev1.Franchise, len(franchises))
	for i, f := range franchises {
		out[i] = toFranchise(f)
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
	return connect.NewResponse(&animev1.GetFranchiseResponse{Franchise: toFranchise(f)}), nil
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
	return connect.NewResponse(&animev1.GetSeriesResponse{
		Series:      toSeries(series),
		FranchiseId: franchiseID,
	}), nil
}

// Search matches franchises and series by title.
func (s *Service) Search(_ context.Context, req *connect.Request[animev1.SearchRequest]) (*connect.Response[animev1.SearchResponse], error) {
	matches := s.store.Search(req.Msg.GetQuery(), int(req.Msg.GetLimit()))
	out := make([]*animev1.SearchResult, len(matches))
	for i, m := range matches {
		out[i] = toSearchResult(m)
	}
	return connect.NewResponse(&animev1.SearchResponse{Results: out}), nil
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
		},
	}), nil
}
