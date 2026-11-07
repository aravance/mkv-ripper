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

// Library is a media server whose collections can be managed by name.
// Movie ids are opaque and only meaningful to the Library that returned them.
type Library interface {
	Name() string
	// Scan asks the server to look for new files.
	Scan(ctx context.Context) error
	// FindMovie returns the server's id for the movie, or ErrNotFound.
	FindMovie(ctx context.Context, q MovieQuery) (string, error)
	// Collections returns the names of all collections on the server.
	Collections(ctx context.Context) ([]string, error)
	// MovieCollections returns the names of the collections containing the movie.
	MovieCollections(ctx context.Context, movieId string) ([]string, error)
	// AddToCollection adds the movie to the named collection, creating it if needed.
	AddToCollection(ctx context.Context, movieId, collection string) error
	// RemoveFromCollection removes the movie from the named collection, if it exists.
	RemoveFromCollection(ctx context.Context, movieId, collection string) error
}
