package api

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	animev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1"
	"github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1/animev1connect"
	"github.com/michael-freling/anime-metadata-db/api/internal/index"
)

// Service implements the public anime.v1.AnimeService Connect handler over a
// Store. Three methods: find series, find releases, open one series.
//
// The browse API anime-metadata-web calls is BrowseService, in browse.go. Both
// read the same Store, so a series the site renders and a series served here
// come from one code path.
type Service struct {
	store   *Store
	version string
}

// compile-time assertion that Service satisfies the generated handler.
var _ animev1connect.AnimeServiceHandler = (*Service)(nil)

// NewService returns a Service backed by store. version is reported by the
// browse API's GetStats.
func NewService(store *Store, version string) *Service {
	return &Service{store: store, version: version}
}

// storeError maps a store failure onto a Connect code. A bad page token is the
// caller's fault; anything else means a record file could not be read or
// parsed, which is ours.
func storeError(err error) *connect.Error {
	if errors.Is(err, index.ErrInvalidPageToken) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

// SearchSeries pages through series matching a keyword. An empty query walks
// the whole catalogue.
//
// It reads no record files: a SeriesSummary is assembled entirely from the
// index, which is what lets the result page stay cheap however deep the series
// it names happen to be.
func (s *Service) SearchSeries(_ context.Context, req *connect.Request[animev1.SearchSeriesRequest]) (*connect.Response[animev1.SearchSeriesResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.SeriesSearch(req.Msg.GetQuery(), req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]*animev1.SeriesSummary, len(page.Items))
	for i, e := range page.Items {
		out[i] = toSeriesSummary(loc, e)
	}
	return connect.NewResponse(&animev1.SearchSeriesResponse{
		Series:        out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// SearchReleases pages through individual releases, filtered by keyword, year
// and quarter in any combination.
//
// A quarter with no year is rejected. Only seasons carry a quarter, so such a
// request would answer with every Winter season the dataset has ever held,
// under a heading the caller meant to be one year — a filter that looks like it
// worked. The year filter has no such ambiguity to guard: proto3 gives it no
// presence, so 0 already means "unfiltered".
func (s *Service) SearchReleases(_ context.Context, req *connect.Request[animev1.SearchReleasesRequest]) (*connect.Response[animev1.SearchReleasesResponse], error) {
	season := req.Msg.GetReleaseSeason()
	if season != animev1.ReleaseSeason_SEASON_UNSPECIFIED && req.Msg.GetReleaseYear() == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("release_season requires release_year: a quarter alone spans every year in the dataset"))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.Works(WorkFilter{
		ReleaseYear:   int(req.Msg.GetReleaseYear()),
		ReleaseSeason: fromReleaseSeason(season),
		Query:         req.Msg.GetQuery(),
	}, req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]*animev1.ReleaseSummary, len(page.Items))
	for i, w := range page.Items {
		out[i] = toReleaseSummary(loc, w)
	}
	return connect.NewResponse(&animev1.SearchReleasesResponse{
		Releases:      out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// GetSeries returns one series by id with everything under it, or CodeNotFound.
//
// Everything means everything: every season with every episode, every film,
// every special, and the whole cast with voice actors resolved. There is no
// companion call to page a truncated collection because nothing is truncated.
func (s *Service) GetSeries(_ context.Context, req *connect.Request[animev1.GetSeriesRequest]) (*connect.Response[animev1.GetSeriesResponse], error) {
	id := req.Msg.GetId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	series, franchiseID, ok, err := s.store.Series(id)
	if err != nil {
		return nil, storeError(err)
	}
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("series %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	out, err := toSeries(loc, s.store, series)
	if err != nil {
		return nil, storeError(err)
	}
	return connect.NewResponse(&animev1.GetSeriesResponse{
		Series:      out,
		FranchiseId: franchiseID,
	}), nil
}
