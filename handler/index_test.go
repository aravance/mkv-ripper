package handler

import (
	"slices"
	"testing"
	"time"

	"github.com/aravance/mkv-ripper/model"
)

func TestCompareMovieGroups_SortsAlphabeticallyIgnoringArticles(t *testing.T) {
	now := time.Now()
	groups := []*model.MovieGroup{
		{Name: "The Zoo", UpdatedAt: now},
		{Name: "Dune", UpdatedAt: now.Add(-time.Hour)},
		{Name: "A Movie", UpdatedAt: now.Add(-2 * time.Hour)},
		{Name: "Batman", UpdatedAt: now.Add(-3 * time.Hour)},
	}

	slices.SortFunc(groups, compareMovieGroups)

	var names []string
	for _, g := range groups {
		names = append(names, g.Name)
	}

	want := []string{"Batman", "Dune", "A Movie", "The Zoo"}
	if !slices.Equal(names, want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
}
