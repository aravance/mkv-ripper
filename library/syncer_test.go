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
	}
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

func (l *memLibrary) Collections(context.Context) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, 0)
	for n := range l.collections {
		names = append(names, n)
	}
	return names, nil
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

func TestSyncerAllCollectionsSorted(t *testing.T) {
	s := NewSyncer(newMemLibrary(), newMemLibrary())
	got, err := s.AllCollections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"Disney", "Kids Movies"}, got); diff != "" {
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
	if err := s.AfterIngest(context.Background(), MovieQuery{ImdbId: "tt1"}, nil); err != nil {
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
