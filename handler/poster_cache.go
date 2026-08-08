package handler

import (
	"sync"

	"github.com/aravance/mkv-ripper/model"
	"github.com/eefret/gomdb"
)

// posterCache memoizes OMDB poster lookups by IMDB id for the life of the
// process, since the history page can list well over a hundred movies and
// their posters never change.
type posterCache struct {
	omdbapi *gomdb.OmdbApi
	mu      sync.Mutex
	byID    map[string]string
}

func newPosterCache(omdbapi *gomdb.OmdbApi) *posterCache {
	return &posterCache{omdbapi: omdbapi, byID: make(map[string]string)}
}

func (c *posterCache) get(imdbId *string) string {
	if imdbId == nil || *imdbId == "" {
		return ""
	}

	c.mu.Lock()
	poster, ok := c.byID[*imdbId]
	c.mu.Unlock()
	if ok {
		return poster
	}

	if movie, err := c.omdbapi.MovieByImdbID(*imdbId); err == nil && movie.Poster != "N/A" {
		poster = movie.Poster
	}

	c.mu.Lock()
	c.byID[*imdbId] = poster
	c.mu.Unlock()
	return poster
}

// fill populates Poster on each group, fetching cache misses concurrently
// since a cold cache on the history page can mean well over a hundred
// sequential OMDB round trips otherwise.
func (c *posterCache) fill(movies []*model.MovieGroup) {
	const maxConcurrent = 8
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, mv := range movies {
		wg.Add(1)
		sem <- struct{}{}
		go func(mv *model.MovieGroup) {
			defer wg.Done()
			defer func() { <-sem }()
			mv.Poster = c.get(mv.ImdbId)
		}(mv)
	}

	wg.Wait()
}
