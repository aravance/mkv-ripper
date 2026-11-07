package handler

import (
	"testing"

	"github.com/aravance/mkv-ripper/library"
	"github.com/aravance/mkv-ripper/model"
	"github.com/google/go-cmp/cmp"
)

func TestWantedCollections(t *testing.T) {
	got := wantedCollections([]string{"Kids Movies", "Disney"}, " Pixar ,, kids movies,Christmas Movies ")
	want := []string{"Kids Movies", "Disney", "Pixar", "Christmas Movies"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("wantedCollections mismatch (-want +got):\n%s", diff)
	}
}

func TestWantedCollectionsEmpty(t *testing.T) {
	got := wantedCollections(nil, "")
	if len(got) != 0 {
		t.Fatalf("expected no collections, got %v", got)
	}
}

func TestSplitCollections(t *testing.T) {
	all := []library.Collection{
		{Name: "The Matrix Collection", Series: true},
		{Name: "Kids Movies"},
		{Name: "Back to the Future Collection", Series: true},
		{Name: "Disney"},
	}
	// "kids movies" exists; "Pixar" doesn't yet, so it's a new topical one.
	topical, series := splitCollections(all, []string{"kids movies", "Pixar", "the matrix collection"})
	if diff := cmp.Diff([]string{"Disney", "Kids Movies", "Pixar"}, topical); diff != "" {
		t.Fatalf("topical mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Back to the Future Collection", "The Matrix Collection"}, series); diff != "" {
		t.Fatalf("series mismatch (-want +got):\n%s", diff)
	}
}

func TestStoredCollectionsUnionsWorkflows(t *testing.T) {
	wfs := []*model.Workflow{
		{DiscId: "a", Collections: []string{"Kids Movies", "Disney"}},
		{DiscId: "b", Collections: []string{"disney", "Pixar"}},
		{DiscId: "c"},
	}
	got := storedCollections(wfs)
	want := []string{"Disney", "Kids Movies", "Pixar"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("storedCollections mismatch (-want +got):\n%s", diff)
	}
}

func TestAnyDone(t *testing.T) {
	if anyDone([]*model.Workflow{{Status: model.StatusPending}, {Status: model.StatusError}}) {
		t.Fatal("expected no done workflows")
	}
	if !anyDone([]*model.Workflow{{Status: model.StatusPending}, {Status: model.StatusDone}}) {
		t.Fatal("expected a done workflow")
	}
}

func TestNeverChosen(t *testing.T) {
	if !neverChosen([]*model.Workflow{{}, {}}) {
		t.Fatal("expected workflows without collections to be never chosen")
	}
	if neverChosen([]*model.Workflow{{}, {Collections: []string{}}}) {
		t.Fatal("an empty, non-nil list means chosen (none on purpose)")
	}
}

var pickerCollections = []library.Collection{
	{Name: "Kids Movies"},
	{Name: "Back to the Future Collection", Series: true, SeriesId: "264"},
	{Name: "Harry Potter Collection", Series: true, SeriesId: "1241"},
	{Name: "Harry Potter Extended Edition", Series: true, SeriesId: "1241"},
}

func TestBuildPickerAutoSelectsUniqueSeries(t *testing.T) {
	sugg := &library.Suggestion{Series: library.Series{TmdbId: "264"}, Matches: []string{"Back to the Future Collection"}}
	p := buildPicker(pickerCollections, []string{"Kids Movies"}, true, sugg)
	if diff := cmp.Diff([]string{"Kids Movies", "Back to the Future Collection"}, p.Selected); diff != "" {
		t.Fatalf("selected mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Back to the Future Collection"}, p.Suggested); diff != "" {
		t.Fatalf("suggested mismatch (-want +got):\n%s", diff)
	}
	if p.NewSeries != "" {
		t.Fatalf("unexpected new series %q", p.NewSeries)
	}
}

func TestBuildPickerRespectsChosenCollections(t *testing.T) {
	sugg := &library.Suggestion{Series: library.Series{TmdbId: "264"}, Matches: []string{"Back to the Future Collection"}}
	p := buildPicker(pickerCollections, []string{}, false, sugg)
	if len(p.Selected) != 0 {
		t.Fatalf("expected nothing pre-selected once chosen, got %v", p.Selected)
	}
	if diff := cmp.Diff([]string{"Back to the Future Collection"}, p.Suggested); diff != "" {
		t.Fatalf("suggestion should still be marked (-want +got):\n%s", diff)
	}
}

func TestBuildPickerAmbiguousSeriesOnlySuggested(t *testing.T) {
	sugg := &library.Suggestion{Series: library.Series{TmdbId: "1241"}, Matches: []string{"Harry Potter Collection", "Harry Potter Extended Edition"}}
	p := buildPicker(pickerCollections, nil, true, sugg)
	if len(p.Selected) != 0 {
		t.Fatalf("expected nothing pre-selected for ambiguous series, got %v", p.Selected)
	}
	if len(p.Suggested) != 2 {
		t.Fatalf("expected both editions suggested, got %v", p.Suggested)
	}
}

func TestBuildPickerOffersNewSeriesCollection(t *testing.T) {
	sugg := &library.Suggestion{Series: library.Series{TmdbId: "2150", Name: "Shrek Collection"}}
	p := buildPicker(pickerCollections, nil, true, sugg)
	if p.NewSeries != "Shrek Collection" {
		t.Fatalf("NewSeries = %q, want Shrek Collection", p.NewSeries)
	}
	if len(p.Selected) != 0 {
		t.Fatalf("a new collection should be offered, not pre-selected: %v", p.Selected)
	}

	// already chosen (e.g. saved before it was created): no offer
	p = buildPicker(pickerCollections, []string{"shrek collection"}, false, sugg)
	if p.NewSeries != "" {
		t.Fatalf("NewSeries = %q, want none when already selected", p.NewSeries)
	}
}

func TestBuildPickerNoSuggestion(t *testing.T) {
	p := buildPicker(pickerCollections, []string{"Kids Movies"}, true, nil)
	if diff := cmp.Diff([]string{"Kids Movies"}, p.Selected); diff != "" {
		t.Fatalf("selected mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"Back to the Future Collection", "Harry Potter Collection", "Harry Potter Extended Edition"}, p.Series); diff != "" {
		t.Fatalf("series mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildPickerNoBadgeWhenAlreadyInSeries(t *testing.T) {
	sugg := &library.Suggestion{Series: library.Series{TmdbId: "264"}, Matches: []string{"Back to the Future Collection"}}
	for _, autoSelect := range []bool{true, false} {
		p := buildPicker(pickerCollections, []string{"Back to the Future Collection"}, autoSelect, sugg)
		if len(p.Suggested) != 0 {
			t.Fatalf("autoSelect=%v: expected no suggestion badge for a series it's already in, got %v", autoSelect, p.Suggested)
		}
		if diff := cmp.Diff([]string{"Back to the Future Collection"}, p.Selected); diff != "" {
			t.Fatalf("selected mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestBuildPickerBadgesOnlyMissingEdition(t *testing.T) {
	sugg := &library.Suggestion{Series: library.Series{TmdbId: "1241"}, Matches: []string{"Harry Potter Collection", "Harry Potter Extended Edition"}}
	p := buildPicker(pickerCollections, []string{"Harry Potter Collection"}, true, sugg)
	if diff := cmp.Diff([]string{"Harry Potter Extended Edition"}, p.Suggested); diff != "" {
		t.Fatalf("suggested mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildPickerHasChanges(t *testing.T) {
	sugg := &library.Suggestion{Series: library.Series{TmdbId: "264"}, Matches: []string{"Back to the Future Collection"}}

	// nothing pre-selected beyond what's saved: nothing to save
	if p := buildPicker(pickerCollections, []string{"Kids Movies"}, false, sugg); p.HasChanges() {
		t.Fatal("expected no changes when selection matches what's saved")
	}
	// an auto-selected series isn't saved yet, so it's a change
	if p := buildPicker(pickerCollections, []string{"Kids Movies"}, true, sugg); !p.HasChanges() {
		t.Fatal("expected a pre-checked suggestion to count as a change")
	}
	// already in the series: nothing to save
	if p := buildPicker(pickerCollections, []string{"back to the future collection"}, true, sugg); p.HasChanges() {
		t.Fatal("expected no changes when already in the series (case-insensitive)")
	}
}
