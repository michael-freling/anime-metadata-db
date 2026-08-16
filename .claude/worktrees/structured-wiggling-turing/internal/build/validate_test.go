package build

import (
	"testing"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

func seriesRecord(s *model.Series) model.Record { return model.Record{Series: s} }

func TestValidateSuccess(t *testing.T) {
	rec := model.Record{Franchise: &model.Franchise{
		ID: "fate",
		Series: []model.Series{{
			ID:      "fate-zero",
			Seasons: []model.Season{{ID: "fz-s1", Number: 1, ReleaseSeason: model.SeasonFall}},
			Movies:  []model.Movie{{ID: "fz-movie", AlternateCutOf: &model.AlternateCutOf{SeasonID: "fz-s1"}}},
			Specials: []model.Special{{
				ID: "fz-ova", Format: model.FormatOVA,
			}},
		}},
		WatchOrders: []model.WatchOrder{{
			Name:    "Chronological",
			Entries: []model.WatchOrderEntry{{Ref: "fate-zero"}, {Ref: "fz-s1"}},
		}},
	}}
	if err := validate(rec); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name string
		rec  model.Record
	}{
		{"franchise no id", model.Record{Franchise: &model.Franchise{Series: []model.Series{{ID: "s"}}}}},
		{"series no id", seriesRecord(&model.Series{})},
		{"season no id", seriesRecord(&model.Series{ID: "s", Seasons: []model.Season{{Number: 1}}})},
		{"season dup id", seriesRecord(&model.Series{ID: "s", Seasons: []model.Season{
			{ID: "x", Number: 1}, {ID: "x", Number: 2},
		}})},
		{"season number < 1", seriesRecord(&model.Series{ID: "s", Seasons: []model.Season{{ID: "x", Number: 0}}})},
		{"bad release season", seriesRecord(&model.Series{ID: "s", Seasons: []model.Season{
			{ID: "x", Number: 1, ReleaseSeason: "AUTUMN"},
		}})},
		{"movie no id", seriesRecord(&model.Series{ID: "s", Movies: []model.Movie{{}}})},
		{"movie dup id", seriesRecord(&model.Series{ID: "s", Movies: []model.Movie{{ID: "m"}, {ID: "m"}}})},
		{"alt cut unknown season", seriesRecord(&model.Series{ID: "s", Movies: []model.Movie{
			{ID: "m", AlternateCutOf: &model.AlternateCutOf{SeasonID: "ghost"}},
		}})},
		{"special no id", seriesRecord(&model.Series{ID: "s", Specials: []model.Special{{}}})},
		{"special dup id", seriesRecord(&model.Series{ID: "s", Specials: []model.Special{{ID: "sp"}, {ID: "sp"}}})},
		{"bad special format", seriesRecord(&model.Series{ID: "s", Specials: []model.Special{
			{ID: "sp", Format: "BOGUS"},
		}})},
		{"id collision across kinds", seriesRecord(&model.Series{ID: "dup", Seasons: []model.Season{{ID: "dup", Number: 1}}})},
		{"watch order no name", model.Record{Franchise: &model.Franchise{ID: "f", Series: []model.Series{{ID: "s"}},
			WatchOrders: []model.WatchOrder{{Entries: []model.WatchOrderEntry{{Ref: "s"}}}}}}},
		{"watch order no entries", model.Record{Franchise: &model.Franchise{ID: "f", Series: []model.Series{{ID: "s"}},
			WatchOrders: []model.WatchOrder{{Name: "X"}}}}},
		{"watch order unknown ref", model.Record{Franchise: &model.Franchise{ID: "f", Series: []model.Series{{ID: "s"}},
			WatchOrders: []model.WatchOrder{{Name: "X", Entries: []model.WatchOrderEntry{{Ref: "ghost"}}}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validate(tt.rec); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestAssignAbsoluteNumbersInterleave(t *testing.T) {
	movieDate := model.NewDate(2019, 6, 15)
	s := &model.Series{
		ID: "rascal",
		Seasons: []model.Season{
			{ID: "s1", Number: 1, ReleaseYear: 2018, ReleaseSeason: model.SeasonFall,
				Episodes: []model.Episode{{AiredNumber: 1}, {AiredNumber: 2}}},
			{ID: "s2", Number: 2, ReleaseYear: 2025, ReleaseSeason: model.SeasonWinter,
				Episodes: []model.Episode{{AiredNumber: 1}}},
		},
		Movies: []model.Movie{
			{ID: "movie", ReleaseDate: &movieDate},
			{ID: "altcut", AlternateCutOf: &model.AlternateCutOf{SeasonID: "s1"}},
		},
	}
	assignAbsoluteNumbers(s)

	if got := *s.Seasons[0].Episodes[0].AbsoluteNumber; got != 1 {
		t.Errorf("s1 ep1 = %d, want 1", got)
	}
	if got := *s.Seasons[0].Episodes[1].AbsoluteNumber; got != 2 {
		t.Errorf("s1 ep2 = %d, want 2", got)
	}
	if got := *s.Movies[0].AbsoluteNumber; got != 3 {
		t.Errorf("interleaved movie = %d, want 3", got)
	}
	if got := *s.Seasons[1].Episodes[0].AbsoluteNumber; got != 4 {
		t.Errorf("s2 ep1 = %d, want 4", got)
	}
	if s.Movies[1].AbsoluteNumber != nil {
		t.Error("alternate cut should not be numbered")
	}
}
