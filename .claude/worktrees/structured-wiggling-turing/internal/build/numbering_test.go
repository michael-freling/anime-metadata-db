package build

import (
	"testing"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// TestAssignAbsoluteNumbersTiebreak exercises the secondary sort keys
// (number, part, input index) when several units share a release key.
func TestAssignAbsoluteNumbersTiebreak(t *testing.T) {
	// Three seasons in the same airing quarter: ordered by number then part.
	s := &model.Series{ID: "x", Seasons: []model.Season{
		{ID: "s2", Number: 2, ReleaseYear: 2012, ReleaseSeason: model.SeasonSpring,
			Episodes: []model.Episode{{AiredNumber: 1}}},
		{ID: "s1b", Number: 1, Part: intp(2), ReleaseYear: 2012, ReleaseSeason: model.SeasonSpring,
			Episodes: []model.Episode{{AiredNumber: 1}}},
		{ID: "s1a", Number: 1, Part: intp(1), ReleaseYear: 2012, ReleaseSeason: model.SeasonSpring,
			Episodes: []model.Episode{{AiredNumber: 1}}},
	}}
	assignAbsoluteNumbers(s)
	got := map[string]int{}
	for _, sea := range s.Seasons {
		got[sea.ID] = *sea.Episodes[0].AbsoluteNumber
	}
	if got["s1a"] != 1 || got["s1b"] != 2 || got["s2"] != 3 {
		t.Errorf("tiebreak order wrong: %+v", got)
	}
}

// TestAssignAbsoluteNumbersIndexTiebreak exercises the final input-index
// tiebreak when units share key, number and part (two same-day movies).
func TestAssignAbsoluteNumbersIndexTiebreak(t *testing.T) {
	d := model.NewDate(2020, 1, 1)
	s := &model.Series{ID: "y", Movies: []model.Movie{
		{ID: "m1", ReleaseDate: &d},
		{ID: "m2", ReleaseDate: &d},
	}}
	assignAbsoluteNumbers(s)
	if *s.Movies[0].AbsoluteNumber != 1 || *s.Movies[1].AbsoluteNumber != 2 {
		t.Errorf("index tiebreak wrong: %v %v", *s.Movies[0].AbsoluteNumber, *s.Movies[1].AbsoluteNumber)
	}
}
