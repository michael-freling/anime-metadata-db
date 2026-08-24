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

// ListFranchises pages through the catalog's franchises, each expanded to its
// full tree.
//
// It is the one listing that reads record files, because a franchise response
// embeds everything beneath it. That is why it pages: unbounded, it would be a
// request to parse the whole dataset into one response. ListCatalog answers
// "what is in here" without the tree.
func (s *Service) ListFranchises(_ context.Context, req *connect.Request[animev1.ListFranchisesRequest]) (*connect.Response[animev1.ListFranchisesResponse], error) {
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.FranchisesPage(req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]*animev1.Franchise, len(page.Items))
	for i, f := range page.Items {
		if out[i], err = toFranchise(loc, s.store, f); err != nil {
			return nil, storeError(err)
		}
	}
	return connect.NewResponse(&animev1.ListFranchisesResponse{
		Franchises:    out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
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

// GetFranchise returns one franchise by id, or CodeNotFound.
func (s *Service) GetFranchise(_ context.Context, req *connect.Request[animev1.GetFranchiseRequest]) (*connect.Response[animev1.GetFranchiseResponse], error) {
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
	return connect.NewResponse(&animev1.GetFranchiseResponse{Franchise: out}), nil
}

// GetSeries returns one series by id (under a franchise or standalone), or
// CodeNotFound.
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
		if !s.store.SeriesExists(seriesID) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("series %q not found", seriesID))
		}
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	filter := WorkFilter{
		ReleaseYear:   int(req.Msg.GetReleaseYear()),
		ReleaseSeason: fromReleaseSeason(req.Msg.GetReleaseSeason()),
		Kind:          fromWorkKind(req.Msg.GetKind()),
		SeriesID:      seriesID,
		Query:         req.Msg.GetQuery(),
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
	c, ok, err := s.store.Character(id)
	if err != nil {
		return nil, storeError(err)
	}
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("character %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	return connect.NewResponse(&animev1.GetCharacterResponse{Character: toCharacter(loc, s.store, "", c)}), nil
}

// ListCharacters returns the whole cast, or one series' cast when series_id is
// set. An unknown series_id is CodeNotFound rather than an empty list, so a
// typo is not mistaken for a series with no cast.
func (s *Service) ListCharacters(_ context.Context, req *connect.Request[animev1.ListCharactersRequest]) (*connect.Response[animev1.ListCharactersResponse], error) {
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
	return connect.NewResponse(&animev1.ListCharactersResponse{
		Characters:    toCharacters(loc, s.store, seriesID, page.Items),
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
	credits, err := s.store.CreditsPage(id, "", index.EmbeddedLimit)
	if err != nil {
		return nil, storeError(err)
	}
	return connect.NewResponse(&animev1.GetStaffResponse{
		Staff:        toStaff(loc, st),
		Credits:      toStaffCredits(loc, s.store, credits.Items),
		CreditsTotal: int32(credits.Total),
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

// ListEpisodes pages the episodes of one season or special. Exactly one parent
// id is required: naming both, or neither, is ambiguous rather than a default.
func (s *Service) ListEpisodes(_ context.Context, req *connect.Request[animev1.ListEpisodesRequest]) (*connect.Response[animev1.ListEpisodesResponse], error) {
	seasonID, specialID := req.Msg.GetSeasonId(), req.Msg.GetSpecialId()
	switch {
	case seasonID == "" && specialID == "":
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("season_id or special_id is required"))
	case seasonID != "" && specialID != "":
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name season_id or special_id, not both"))
	}
	id := seasonID + specialID
	if !s.store.WorkExists(id) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("%q not found", id))
	}
	page, err := s.store.EpisodesPage(seasonID, specialID, req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	return connect.NewResponse(&animev1.ListEpisodesResponse{
		Episodes:      toEpisodes(page.Items),
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// ListSeries pages one franchise's series. The flat catalog of everything is
// ListCatalog; this answers "what is under this brand".
func (s *Service) ListSeries(_ context.Context, req *connect.Request[animev1.ListSeriesRequest]) (*connect.Response[animev1.ListSeriesResponse], error) {
	id := req.Msg.GetFranchiseId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("franchise_id is required"))
	}
	if _, ok := s.store.FranchiseExists(id); !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("franchise %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.SeriesPage(id, req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]*animev1.Series, len(page.Items))
	for i, series := range page.Items {
		if out[i], err = toSeries(loc, s.store, series); err != nil {
			return nil, storeError(err)
		}
	}
	return connect.NewResponse(&animev1.ListSeriesResponse{
		Series:        out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// ListAppearances pages the series one character appears in.
func (s *Service) ListAppearances(_ context.Context, req *connect.Request[animev1.ListAppearancesRequest]) (*connect.Response[animev1.ListAppearancesResponse], error) {
	id := req.Msg.GetCharacterId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("character_id is required"))
	}
	// Resolved once: the appearances to page and the cast that holds
	// throughout, which each of them is resolved against, both live on the
	// character. The not-found check comes from this read rather than a
	// separate index probe, so there is no window where one says yes and the
	// other returns nothing to page.
	character, ok, err := s.store.Character(id)
	if err != nil {
		return nil, storeError(err)
	}
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("character %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.AppearancesPage(character, req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	out := make([]*animev1.CharacterAppearance, len(page.Items))
	for i := range page.Items {
		out[i] = toAppearance(loc, s.store, character.VoiceActors, page.Items[i])
	}
	return connect.NewResponse(&animev1.ListAppearancesResponse{
		Appearances:   out,
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}

// ListCredits pages the roles one staff member is cast in.
func (s *Service) ListCredits(_ context.Context, req *connect.Request[animev1.ListCreditsRequest]) (*connect.Response[animev1.ListCreditsResponse], error) {
	id := req.Msg.GetStaffId()
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("staff_id is required"))
	}
	if _, ok := s.store.Staff(id); !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("staff %q not found", id))
	}
	loc := newLocalizer(req.Header().Get("Accept-Language"))
	page, err := s.store.CreditsPage(id, req.Msg.GetPageToken(), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, storeError(err)
	}
	return connect.NewResponse(&animev1.ListCreditsResponse{
		Credits:       toStaffCredits(loc, s.store, page.Items),
		NextPageToken: page.NextToken,
		TotalSize:     int32(page.Total),
	}), nil
}
