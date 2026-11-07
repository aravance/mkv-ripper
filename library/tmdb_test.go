package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fakeTmdb serves /find and /movie for two movies: Back to the Future (in
// series 264) and Napoleon Dynamite (in no series).
func fakeTmdb(t *testing.T, checkAuth func(*http.Request) bool) (*httptest.Server, *atomic.Int32) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if !checkAuth(r) {
			http.Error(w, `{"status_message":"Invalid API key"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/find/tt0088763":
			w.Write([]byte(`{"movie_results":[{"id":105,"title":"Back to the Future"}]}`))
		case "/find/tt0374900":
			w.Write([]byte(`{"movie_results":[{"id":8193}]}`))
		case "/find/tt0000000":
			w.Write([]byte(`{"movie_results":[]}`))
		case "/movie/105":
			w.Write([]byte(`{"id":105,"belongs_to_collection":{"id":264,"name":"Back to the Future Collection"}}`))
		case "/movie/8193":
			w.Write([]byte(`{"id":8193,"belongs_to_collection":null}`))
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func newTestTmdb(url, key string) *Tmdb {
	t := NewTmdb(key)
	t.baseURL = url
	return t
}

func TestTmdbSeriesForMovieWithV3Key(t *testing.T) {
	srv, requests := fakeTmdb(t, func(r *http.Request) bool {
		return r.URL.Query().Get("api_key") == "v3key" && r.Header.Get("Authorization") == ""
	})
	tm := newTestTmdb(srv.URL, "v3key")
	got, err := tm.SeriesForMovie(context.Background(), "tt0088763")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&Series{TmdbId: "264", Name: "Back to the Future Collection"}, got); diff != "" {
		t.Fatalf("series mismatch (-want +got):\n%s", diff)
	}

	// cached: no more requests
	before := requests.Load()
	if _, err := tm.SeriesForMovie(context.Background(), "tt0088763"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != before {
		t.Fatal("expected cached result")
	}
}

func TestTmdbSeriesForMovieWithV4Token(t *testing.T) {
	srv, _ := fakeTmdb(t, func(r *http.Request) bool {
		return r.Header.Get("Authorization") == "Bearer eyJtoken" && r.URL.Query().Get("api_key") == ""
	})
	tm := newTestTmdb(srv.URL, "eyJtoken")
	if got, err := tm.SeriesForMovie(context.Background(), "tt0088763"); err != nil || got == nil {
		t.Fatalf("SeriesForMovie = %v, %v", got, err)
	}
}

func TestTmdbSeriesForMovieNoSeries(t *testing.T) {
	srv, requests := fakeTmdb(t, func(*http.Request) bool { return true })
	tm := newTestTmdb(srv.URL, "k")
	for _, imdb := range []string{"tt0374900", "tt0000000", "tt0374900"} {
		if got, err := tm.SeriesForMovie(context.Background(), imdb); err != nil || got != nil {
			t.Fatalf("SeriesForMovie(%s) = %v, %v; want nil, nil", imdb, got, err)
		}
	}
	if n := requests.Load(); n != 3 {
		t.Fatalf("expected 3 requests (no-series results cached), got %d", n)
	}
}

func TestTmdbErrorDoesNotLeakKey(t *testing.T) {
	srv, _ := fakeTmdb(t, func(*http.Request) bool { return false })
	tm := newTestTmdb(srv.URL, "secretkey")
	_, err := tm.SeriesForMovie(context.Background(), "tt0088763")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
	if strings.Contains(err.Error(), "secretkey") {
		t.Fatalf("error leaks api key: %v", err)
	}

	tm = newTestTmdb("http://127.0.0.1:1", "secretkey")
	if _, err := tm.SeriesForMovie(context.Background(), "tt0088763"); err == nil || strings.Contains(err.Error(), "secretkey") {
		t.Fatalf("expected connection error without key, got %v", err)
	}
}
