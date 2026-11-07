package library

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// memLibrary is an in-memory Library. Movies only become findable once
// visibleAfterFinds lookups have happened, simulating a server scan.
type memLibrary struct {
	mu                sync.Mutex
	imdb              map[string]string   // imdb id -> movie id
	collections       map[string][]string // name -> movie ids
	series            map[string]string   // collection name -> TMDb series id
	movieSeries       map[string]string   // movie id -> TMDb series id
	visibleAfterFinds int
	finds             int
	scans             int
}

func newMemLibrary() *memLibrary {
	return &memLibrary{
		imdb: map[string]string{"tt1": "m1"},
		collections: map[string][]string{
			"Kids Movies": {"m1"},
			"Disney":      {},
		},
		series:      map[string]string{},
		movieSeries: map[string]string{},
	}
}

func (l *memLibrary) MovieSeriesId(_ context.Context, movieId string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.movieSeries[movieId], nil
}

func (l *memLibrary) Name() string { return "mem" }

func (l *memLibrary) Scan(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.scans++
	return nil
}

func (l *memLibrary) FindMovie(_ context.Context, q MovieQuery) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.finds++
	if l.finds <= l.visibleAfterFinds {
		return "", ErrNotFound
	}
	if id, ok := l.imdb[q.ImdbId]; ok {
		return id, nil
	}
	return "", ErrNotFound
}

func (l *memLibrary) Collections(context.Context) ([]Collection, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cols := make([]Collection, 0)
	for n := range l.collections {
		cols = append(cols, Collection{Name: n, Series: l.series[n] != "", SeriesId: l.series[n]})
	}
	return cols, nil
}

func (l *memLibrary) MovieCollections(_ context.Context, movieId string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, 0)
	for n, ids := range l.collections {
		if slices.Contains(ids, movieId) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names, nil
}

func (l *memLibrary) find(name string) string {
	for n := range l.collections {
		if strings.EqualFold(n, name) {
			return n
		}
	}
	return ""
}

func (l *memLibrary) AddToCollection(_ context.Context, movieId, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := l.find(name); n != "" {
		name = n
	}
	l.collections[name] = append(l.collections[name], movieId)
	return nil
}

func (l *memLibrary) RemoveFromCollection(_ context.Context, movieId, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := l.find(name); n != "" {
		l.collections[n] = slices.DeleteFunc(l.collections[n], func(id string) bool { return id == movieId })
	}
	return nil
}

func TestSyncerApplyAddsOnlyByDefault(t *testing.T) {
	lib := newMemLibrary()
	s := NewSyncer(lib)
	if err := s.Apply(context.Background(), MovieQuery{ImdbId: "tt1"}, []string{"disney", "Pixar"}, false); err != nil {
		t.Fatal(err)
	}
	got, _ := lib.MovieCollections(context.Background(), "m1")
	if diff := cmp.Diff([]string{"Disney", "Kids Movies", "Pixar"}, got); diff != "" {
		t.Fatalf("collections mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncerApplyWithRemove(t *testing.T) {
	lib := newMemLibrary()
	s := NewSyncer(lib)
	if err := s.Apply(context.Background(), MovieQuery{ImdbId: "tt1"}, []string{"Disney"}, true); err != nil {
		t.Fatal(err)
	}
	got, _ := lib.MovieCollections(context.Background(), "m1")
	if diff := cmp.Diff([]string{"Disney"}, got); diff != "" {
		t.Fatalf("collections mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncerApplyNotFound(t *testing.T) {
	s := NewSyncer(newMemLibrary())
	if err := s.Apply(context.Background(), MovieQuery{ImdbId: "tt404"}, []string{"Disney"}, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSyncerMembership(t *testing.T) {
	s := NewSyncer(newMemLibrary())
	names, found, err := s.Membership(context.Background(), MovieQuery{ImdbId: "tt1"})
	if err != nil || !found {
		t.Fatalf("Membership: found=%v err=%v", found, err)
	}
	if diff := cmp.Diff([]string{"Kids Movies"}, names); diff != "" {
		t.Fatalf("membership mismatch (-want +got):\n%s", diff)
	}
	if _, found, _ := s.Membership(context.Background(), MovieQuery{ImdbId: "tt404"}); found {
		t.Fatal("expected unknown movie to be not found")
	}
}

func TestSyncerAllCollectionsMergesAndSorts(t *testing.T) {
	a, b := newMemLibrary(), newMemLibrary()
	a.collections["The Matrix Collection"] = nil
	b.collections["the matrix collection"] = nil
	b.series["the matrix collection"] = "2344"
	s := NewSyncer(a, b)
	got, err := s.AllCollections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Collection{
		{Name: "Disney"},
		{Name: "Kids Movies"},
		{Name: "The Matrix Collection", Series: true, SeriesId: "2344"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("AllCollections mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncerAfterIngestWaitsForScan(t *testing.T) {
	lib := newMemLibrary()
	lib.visibleAfterFinds = 3
	s := NewSyncer(lib)
	s.PollInterval = time.Millisecond
	s.RescanInterval = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.AfterIngest(ctx, MovieQuery{ImdbId: "tt1"}, []string{"Disney"}); err != nil {
		t.Fatal(err)
	}
	got, _ := lib.MovieCollections(context.Background(), "m1")
	if diff := cmp.Diff([]string{"Disney", "Kids Movies"}, got); diff != "" {
		t.Fatalf("collections mismatch (-want +got):\n%s", diff)
	}
	if lib.scans != 1 || lib.finds != 4 {
		t.Fatalf("scans=%d finds=%d, want 1 scan and 4 finds", lib.scans, lib.finds)
	}
}

func TestSyncerAfterIngestNoCollectionsOnlyScans(t *testing.T) {
	lib := newMemLibrary()
	s := NewSyncer(lib)
	if err := s.AfterIngest(context.Background(), MovieQuery{ImdbId: "tt1"}, []string{}); err != nil {
		t.Fatal(err)
	}
	if lib.scans != 1 || lib.finds != 0 {
		t.Fatalf("scans=%d finds=%d, want 1 scan and no finds", lib.scans, lib.finds)
	}
}

func TestSyncerAfterIngestGivesUp(t *testing.T) {
	lib := newMemLibrary()
	s := NewSyncer(lib)
	s.PollInterval = time.Millisecond
	s.RescanInterval = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := s.AfterIngest(ctx, MovieQuery{ImdbId: "tt404"}, []string{"Disney"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if lib.scans < 2 {
		t.Fatalf("expected rescans while waiting, got %d scans", lib.scans)
	}
}

func TestSyncerDisabled(t *testing.T) {
	var nilSyncer *Syncer
	if nilSyncer.Enabled() || NewSyncer().Enabled() {
		t.Fatal("expected syncer without libraries to be disabled")
	}
}

type fakeLookup struct {
	series *Series
	err    error
	calls  int
}

func (f *fakeLookup) SeriesForMovie(context.Context, string) (*Series, error) {
	f.calls++
	return f.series, f.err
}

// seriesLibrary has m1 (tt1) in TMDb series 264, which has one collection.
func seriesLibrary() *memLibrary {
	lib := newMemLibrary()
	lib.collections["Back to the Future Collection"] = nil
	lib.series["Back to the Future Collection"] = "264"
	lib.movieSeries["m1"] = "264"
	return lib
}

func TestSyncerSuggestSeriesFromLookup(t *testing.T) {
	lib := seriesLibrary()
	s := NewSyncer(lib)
	s.Lookup = &fakeLookup{series: &Series{TmdbId: "264", Name: "Back to the Future Collection"}}
	all, _ := s.AllCollections(context.Background())
	got, err := s.SuggestSeries(context.Background(), MovieQuery{ImdbId: "tt404"}, all)
	if err != nil {
		t.Fatal(err)
	}
	want := &Suggestion{Series: Series{TmdbId: "264", Name: "Back to the Future Collection"}, Matches: []string{"Back to the Future Collection"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("suggestion mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncerSuggestSeriesFallsBackToLibrary(t *testing.T) {
	lib := seriesLibrary()
	s := NewSyncer(lib)
	s.Lookup = &fakeLookup{err: errors.New("tmdb down")}
	all, _ := s.AllCollections(context.Background())
	got, err := s.SuggestSeries(context.Background(), MovieQuery{ImdbId: "tt1"}, all)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Series.TmdbId != "264" || len(got.Matches) != 1 {
		t.Fatalf("suggestion = %+v, want series 264 with one match", got)
	}
}

func TestSyncerSuggestSeriesNone(t *testing.T) {
	s := NewSyncer(newMemLibrary())
	s.Lookup = &fakeLookup{}
	got, err := s.SuggestSeries(context.Background(), MovieQuery{ImdbId: "tt1"}, nil)
	if err != nil || got != nil {
		t.Fatalf("SuggestSeries = %+v, %v; want nil, nil", got, err)
	}
}

func TestSyncerAfterIngestAutoAddsUniqueSeries(t *testing.T) {
	lib := seriesLibrary()
	s := NewSyncer(lib)
	if err := s.AfterIngest(context.Background(), MovieQuery{ImdbId: "tt1"}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := lib.MovieCollections(context.Background(), "m1")
	if diff := cmp.Diff([]string{"Back to the Future Collection", "Kids Movies"}, got); diff != "" {
		t.Fatalf("collections mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncerAfterIngestSkipsAmbiguousSeries(t *testing.T) {
	lib := seriesLibrary()
	lib.collections["Back to the Future Extended Edition"] = nil
	lib.series["Back to the Future Extended Edition"] = "264"
	s := NewSyncer(lib)
	if err := s.AfterIngest(context.Background(), MovieQuery{ImdbId: "tt1"}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := lib.MovieCollections(context.Background(), "m1")
	if diff := cmp.Diff([]string{"Kids Movies"}, got); diff != "" {
		t.Fatalf("collections mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncerAfterIngestChosenCollectionsSkipSeries(t *testing.T) {
	lib := seriesLibrary()
	s := NewSyncer(lib)
	if err := s.AfterIngest(context.Background(), MovieQuery{ImdbId: "tt1"}, []string{"Disney"}); err != nil {
		t.Fatal(err)
	}
	got, _ := lib.MovieCollections(context.Background(), "m1")
	if diff := cmp.Diff([]string{"Disney", "Kids Movies"}, got); diff != "" {
		t.Fatalf("collections mismatch (-want +got):\n%s", diff)
	}
}

func TestSyncerAfterIngestUsesLookupWhenLibraryLacksSeries(t *testing.T) {
	lib := seriesLibrary()
	delete(lib.movieSeries, "m1")
	s := NewSyncer(lib)
	s.Lookup = &fakeLookup{series: &Series{TmdbId: "264"}}
	if err := s.AfterIngest(context.Background(), MovieQuery{ImdbId: "tt1"}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := lib.MovieCollections(context.Background(), "m1")
	if !ContainsCollection(got, "Back to the Future Collection") {
		t.Fatalf("expected series collection, got %v", got)
	}
}

func TestSyncerSuggestSeriesWithoutLookup(t *testing.T) {
	lib := seriesLibrary()
	s := NewSyncer(lib) // no TMDb configured
	all, _ := s.AllCollections(context.Background())

	got, err := s.SuggestSeries(context.Background(), MovieQuery{ImdbId: "tt1"}, all)
	if err != nil {
		t.Fatal(err)
	}
	want := &Suggestion{Series: Series{TmdbId: "264"}, Matches: []string{"Back to the Future Collection"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("suggestion mismatch (-want +got):\n%s", diff)
	}

	// not in any library yet (e.g. before the rip): no suggestion, no error
	got, err = s.SuggestSeries(context.Background(), MovieQuery{ImdbId: "tt404"}, all)
	if err != nil || got != nil {
		t.Fatalf("SuggestSeries before import = %+v, %v; want nil, nil", got, err)
	}
}
