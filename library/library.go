// Package library talks to media servers (Jellyfin today, Plex later) that
// serve the movies mkv-ripper ingests, so a freshly ingested movie can be
// added to the collections chosen for it.
package library

import (
	"context"
	"errors"

	"github.com/aravance/mkv-ripper/model"
)

// ErrNotFound is returned when a movie isn't (yet) known to a library, e.g.
// because the server hasn't scanned the newly ingested file.
var ErrNotFound = errors.New("movie not found in library")

// MovieQuery identifies a movie on a media server. ImdbId is preferred when
// set; Name and Year are used to narrow the search and as a fallback match.
type MovieQuery struct {
	ImdbId string
	Name   string
	Year   string
}

func QueryForWorkflow(wf *model.Workflow) MovieQuery {
	var q MovieQuery
	if wf.ImdbId != nil {
		q.ImdbId = *wf.ImdbId
	}
	if wf.Name != nil {
		q.Name = *wf.Name
	}
	if wf.Year != nil {
		q.Year = *wf.Year
	}
	return q
}

// Collection is a named collection on a media server.
type Collection struct {
	Name string
	// Series marks a release series (a franchise or trilogy, e.g. "Back to
	// the Future Collection") as opposed to a topical collection (e.g.
	// "Christmas Movies"). Jellyfin identifies series against TMDb's
	// collections, so a TMDb id means it's a series.
	Series bool
	// SeriesId is the series' TMDb collection id, when known.
	SeriesId string
}

// Series is a release series a movie belongs to, as identified by TMDb.
type Series struct {
	TmdbId string
	// Name is TMDb's name for the series, e.g. "Shrek Collection". It may
	// be empty when the series was identified by a library instead.
	Name string
}

// SeriesLookup identifies the series a movie belongs to before it's in any
// library, e.g. via the TMDb API. Returns nil if the movie isn't in a series.
type SeriesLookup interface {
	SeriesForMovie(ctx context.Context, imdbId string) (*Series, error)
}

// Library is a media server whose collections can be managed by name.
// Movie ids are opaque and only meaningful to the Library that returned them.
type Library interface {
	Name() string
	// Scan asks the server to look for new files.
	Scan(ctx context.Context) error
	// FindMovie returns the server's id for the movie, or ErrNotFound.
	FindMovie(ctx context.Context, q MovieQuery) (string, error)
	// Collections returns all collections on the server.
	Collections(ctx context.Context) ([]Collection, error)
	// MovieSeriesId returns the TMDb collection id of the series the server
	// thinks the movie belongs to, or "" if none (or unsupported).
	MovieSeriesId(ctx context.Context, movieId string) (string, error)
	// MovieCollections returns the names of the collections containing the movie.
	MovieCollections(ctx context.Context, movieId string) ([]string, error)
	// AddToCollection adds the movie to the named collection, creating it if needed.
	AddToCollection(ctx context.Context, movieId, collection string) error
	// RemoveFromCollection removes the movie from the named collection, if it exists.
	RemoveFromCollection(ctx context.Context, movieId, collection string) error
}
