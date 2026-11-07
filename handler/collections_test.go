package handler

import (
	"testing"

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

func TestCollectionOptionsIncludesUncreatedSelections(t *testing.T) {
	got := collectionOptions([]string{"The Matrix Collection", "Kids Movies"}, []string{"kids movies", "Pixar"})
	want := []string{"Kids Movies", "Pixar", "The Matrix Collection"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("collectionOptions mismatch (-want +got):\n%s", diff)
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
