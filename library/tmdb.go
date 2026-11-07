package library

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const tmdbBaseURL = "https://api.themoviedb.org/3"

// Tmdb is a SeriesLookup backed by the TMDb API. It accepts either a v3 API
// key or a v4 API read access token (the long "eyJ..." one), and caches
// results since the picker asks for the same movie on every page load.
type Tmdb struct {
	baseURL string
	key     string
	client  *http.Client

	mu    sync.Mutex
	cache map[string]*Series
}

func NewTmdb(key string) *Tmdb {
	return &Tmdb{
		baseURL: tmdbBaseURL,
		key:     key,
		client:  &http.Client{Timeout: 15 * time.Second},
		cache:   make(map[string]*Series),
	}
}

func (t *Tmdb) get(ctx context.Context, path string, query url.Values, out any) error {
	if query == nil {
		query = url.Values{}
	}
	// v4 read access tokens are JWTs and go in a header; v3 keys are a param.
	bearer := strings.HasPrefix(t.key, "eyJ")
	if !bearer {
		query.Set("api_key", t.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	if bearer {
		req.Header.Set("Authorization", "Bearer "+t.key)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		// don't leak the api_key query param into logs
		if ue, ok := err.(*url.Error); ok {
			return fmt.Errorf("tmdb GET %s: %w", path, ue.Err)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("tmdb GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (t *Tmdb) SeriesForMovie(ctx context.Context, imdbId string) (*Series, error) {
	if imdbId == "" {
		return nil, nil
	}
	t.mu.Lock()
	cached, ok := t.cache[imdbId]
	t.mu.Unlock()
	if ok {
		return cached, nil
	}

	var found struct {
		MovieResults []struct{ Id int } `json:"movie_results"`
	}
	if err := t.get(ctx, "/find/"+url.PathEscape(imdbId), url.Values{"external_source": {"imdb_id"}}, &found); err != nil {
		return nil, err
	}

	var series *Series
	if len(found.MovieResults) > 0 {
		var movie struct {
			BelongsToCollection *struct {
				Id   int
				Name string
			} `json:"belongs_to_collection"`
		}
		if err := t.get(ctx, "/movie/"+strconv.Itoa(found.MovieResults[0].Id), nil, &movie); err != nil {
			return nil, err
		}
		if c := movie.BelongsToCollection; c != nil {
			series = &Series{TmdbId: strconv.Itoa(c.Id), Name: c.Name}
		}
	}

	t.mu.Lock()
	t.cache[imdbId] = series
	t.mu.Unlock()
	return series, nil
}
