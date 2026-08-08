package handler

import (
	"testing"
	"time"

	"github.com/aravance/mkv-ripper/model"
)

func strPtr(s string) *string { return &s }

func TestGroupMovies_GroupsByImdbId(t *testing.T) {
	now := time.Now()
	wfs := []*model.Workflow{
		{
			DiscId: "disc-4k", TitleId: 0, Status: model.StatusDone,
			ImdbId: strPtr("tt1"), Name: strPtr("Movie One"), Year: strPtr("2020"),
			Resolution: strPtr("4k"), UpdatedAt: now,
		},
		{
			DiscId: "disc-1080p", TitleId: 0, Status: model.StatusDone,
			ImdbId: strPtr("tt1"), Name: strPtr("Movie One"), Year: strPtr("2020"),
			Resolution: strPtr("1080p"), UpdatedAt: now.Add(-time.Hour),
		},
	}

	groups := groupMovies(wfs)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Workflows) != 2 {
		t.Fatalf("expected 2 workflows in group, got %d", len(groups[0].Workflows))
	}
	if *groups[0].Workflows[0].Resolution != "4k" || *groups[0].Workflows[1].Resolution != "1080p" {
		t.Fatalf("expected 4k before 1080p, got %v", groups[0].Workflows)
	}
	if !groups[0].UpdatedAt.Equal(now) {
		t.Fatalf("expected group UpdatedAt to be the latest workflow time")
	}
}

func TestGroupMovies_SeparatesDifferentMovies(t *testing.T) {
	wfs := []*model.Workflow{
		{DiscId: "d1", TitleId: 0, Status: model.StatusDone, ImdbId: strPtr("tt1"), Name: strPtr("A"), Year: strPtr("2020")},
		{DiscId: "d2", TitleId: 0, Status: model.StatusDone, ImdbId: strPtr("tt2"), Name: strPtr("B"), Year: strPtr("2021")},
	}

	groups := groupMovies(wfs)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

func TestGroupMovies_SortedByRecency(t *testing.T) {
	now := time.Now()
	wfs := []*model.Workflow{
		{DiscId: "d1", TitleId: 0, Status: model.StatusDone, ImdbId: strPtr("tt1"), Name: strPtr("Older"), UpdatedAt: now.Add(-time.Hour)},
		{DiscId: "d2", TitleId: 0, Status: model.StatusDone, ImdbId: strPtr("tt2"), Name: strPtr("Newer"), UpdatedAt: now},
	}

	groups := groupMovies(wfs)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Name != "Newer" || groups[1].Name != "Older" {
		t.Fatalf("expected newest first, got %s then %s", groups[0].Name, groups[1].Name)
	}
}

func TestGroupMovies_FallsBackToNameWhenImdbIdMissing(t *testing.T) {
	wfs := []*model.Workflow{
		{DiscId: "d1", TitleId: 0, Status: model.StatusDone, Name: strPtr("No Imdb"), Year: strPtr("2020")},
		{DiscId: "d2", TitleId: 0, Status: model.StatusDone, Name: strPtr("No Imdb"), Year: strPtr("2020")},
	}

	groups := groupMovies(wfs)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Workflows) != 2 {
		t.Fatalf("expected 2 workflows in group, got %d", len(groups[0].Workflows))
	}
}
