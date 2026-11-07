package library

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const testToken = "test-token"

// fakeJellyfin implements just enough of the Jellyfin API for the client,
// including its quirks: AnyProviderIdEquals is ignored, and collection
// members are only listed when a userId is given.
type fakeJellyfin struct {
	t        *testing.T
	mu       sync.Mutex
	movies   []jfItem
	sets     []jfItem
	members  map[string][]string // set id -> movie ids
	scans    int
	nextSet  int
	requests []string
}

func newFakeJellyfin(t *testing.T) (*fakeJellyfin, *Jellyfin) {
	f := &fakeJellyfin{
		t: t,
		movies: []jfItem{
			{Id: "m1", Name: "Back to the Future", ProductionYear: 1985, ProviderIds: map[string]string{"Imdb": "tt0088763", "Tmdb": "105"}},
			{Id: "m2", Name: "Harry Potter and the Philosopher's Stone", ProductionYear: 2001, ProviderIds: map[string]string{"Imdb": "tt0241527"}},
			{Id: "m3", Name: "13 Going on 30", ProductionYear: 2004, ProviderIds: map[string]string{"Imdb": "tt0337563"}},
		},
		sets: []jfItem{
			{Id: "s1", Name: "Back to the Future Collection"},
			{Id: "s2", Name: "Kids Movies"},
		},
		members: map[string][]string{
			"s1": {"m1"},
			"s2": {"m1", "m2"},
		},
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, NewJellyfin(srv.URL+"/", testToken, "")
}

func (f *fakeJellyfin) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (f *fakeJellyfin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)

	if r.Header.Get("Authorization") != `MediaBrowser Token="`+testToken+`"` {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/Users":
		f.writeJSON(w, []map[string]any{
			{"Id": "u-guest", "Name": "guest", "Policy": map[string]any{"IsAdministrator": false}},
			{"Id": "u-admin", "Name": "admin", "Policy": map[string]any{"IsAdministrator": true}},
		})

	case r.Method == http.MethodPost && r.URL.Path == "/Library/Refresh":
		f.scans++
		w.WriteHeader(http.StatusNoContent)

	case r.Method == http.MethodGet && r.URL.Path == "/Items":
		var items []jfItem
		switch {
		case q.Get("ParentId") != "":
			if q.Get("userId") == "" {
				break // real Jellyfin returns no children without a user
			}
			for _, id := range f.members[q.Get("ParentId")] {
				items = append(items, jfItem{Id: id})
			}
		case q.Get("IncludeItemTypes") == "BoxSet":
			items = f.sets
		case q.Get("IncludeItemTypes") == "Movie":
			term := strings.ToLower(q.Get("searchTerm"))
			for _, m := range f.movies {
				if term != "" && !strings.Contains(strings.ToLower(m.Name), term) {
					continue
				}
				items = append(items, m)
			}
		default:
			f.t.Errorf("unexpected /Items query: %v", q)
		}
		f.writeJSON(w, jfItems{Items: items, TotalRecordCount: len(items)})

	case r.Method == http.MethodPost && r.URL.Path == "/Collections":
		f.nextSet++
		id := "new" + string(rune('0'+f.nextSet))
		f.sets = append(f.sets, jfItem{Id: id, Name: q.Get("Name")})
		f.members[id] = strings.Split(q.Get("Ids"), ",")
		f.writeJSON(w, map[string]string{"Id": id})

	case strings.HasPrefix(r.URL.Path, "/Collections/") && strings.HasSuffix(r.URL.Path, "/Items"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/Collections/"), "/Items")
		movie := q.Get("Ids")
		switch r.Method {
		case http.MethodPost:
			if !slices.Contains(f.members[id], movie) {
				f.members[id] = append(f.members[id], movie)
			}
		case http.MethodDelete:
			f.members[id] = slices.DeleteFunc(f.members[id], func(m string) bool { return m == movie })
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	}
}

func TestJellyfinFindMovieByImdbFromSearch(t *testing.T) {
	_, jf := newFakeJellyfin(t)
	id, err := jf.FindMovie(context.Background(), MovieQuery{ImdbId: "tt0088763", Name: "Back to the Future", Year: "1985"})
	if err != nil || id != "m1" {
		t.Fatalf("FindMovie = %q, %v; want m1", id, err)
	}
}

func TestJellyfinFindMovieFallsBackToFullScanWhenTitlesDiffer(t *testing.T) {
	_, jf := newFakeJellyfin(t)
	// OMDb calls it "Sorcerer's Stone", Jellyfin/TMDb "Philosopher's Stone".
	id, err := jf.FindMovie(context.Background(), MovieQuery{ImdbId: "tt0241527", Name: "Harry Potter and the Sorcerer's Stone", Year: "2001"})
	if err != nil || id != "m2" {
		t.Fatalf("FindMovie = %q, %v; want m2", id, err)
	}
}

func TestJellyfinFindMovieByTitleWithoutImdb(t *testing.T) {
	_, jf := newFakeJellyfin(t)
	id, err := jf.FindMovie(context.Background(), MovieQuery{Name: "13 going on 30", Year: "2004"})
	if err != nil || id != "m3" {
		t.Fatalf("FindMovie = %q, %v; want m3", id, err)
	}
	if _, err := jf.FindMovie(context.Background(), MovieQuery{Name: "13 Going on 30", Year: "1999"}); err != ErrNotFound {
		t.Fatalf("FindMovie with wrong year: err = %v, want ErrNotFound", err)
	}
}

func TestJellyfinFindMovieNotFound(t *testing.T) {
	_, jf := newFakeJellyfin(t)
	if _, err := jf.FindMovie(context.Background(), MovieQuery{ImdbId: "tt9999999", Name: "Nope", Year: "2020"}); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestJellyfinMovieCollections(t *testing.T) {
	_, jf := newFakeJellyfin(t)
	got, err := jf.MovieCollections(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"Back to the Future Collection", "Kids Movies"}, got); diff != "" {
		t.Fatalf("MovieCollections mismatch (-want +got):\n%s", diff)
	}
	got, err = jf.MovieCollections(context.Background(), "m3")
	if err != nil || len(got) != 0 {
		t.Fatalf("MovieCollections(m3) = %v, %v; want none", got, err)
	}
}

func TestJellyfinAddToExistingAndNewCollection(t *testing.T) {
	f, jf := newFakeJellyfin(t)
	ctx := context.Background()
	if err := jf.AddToCollection(ctx, "m3", "kids movies"); err != nil {
		t.Fatal(err)
	}
	if err := jf.AddToCollection(ctx, "m3", "Rom Coms"); err != nil {
		t.Fatal(err)
	}
	got, err := jf.MovieCollections(ctx, "m3")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"Kids Movies", "Rom Coms"}, got); diff != "" {
		t.Fatalf("MovieCollections mismatch (-want +got):\n%s", diff)
	}
	if len(f.sets) != 3 {
		t.Fatalf("expected exactly one new collection, sets = %v", f.sets)
	}
}

func TestJellyfinRemoveFromCollection(t *testing.T) {
	f, jf := newFakeJellyfin(t)
	ctx := context.Background()
	if err := jf.RemoveFromCollection(ctx, "m1", "Kids Movies"); err != nil {
		t.Fatal(err)
	}
	if err := jf.RemoveFromCollection(ctx, "m1", "Does Not Exist"); err != nil {
		t.Fatalf("removing from a missing collection should be a no-op, got %v", err)
	}
	if diff := cmp.Diff([]string{"m2"}, f.members["s2"]); diff != "" {
		t.Fatalf("members mismatch (-want +got):\n%s", diff)
	}
}

func TestJellyfinUsesConfiguredUser(t *testing.T) {
	_, jf := newFakeJellyfin(t)
	jf.user = "nobody"
	if _, err := jf.MovieCollections(context.Background(), "m1"); err == nil {
		t.Fatal("expected error for unknown user")
	}
}

func TestJellyfinErrorStatus(t *testing.T) {
	_, jf := newFakeJellyfin(t)
	jf.token = "wrong"
	err := jf.Scan(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}
