package library

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"time"
)

// Syncer applies a movie's chosen collections, by name, across every
// configured Library.
type Syncer struct {
	libs []Library

	// Lookup optionally identifies a movie's series before it's in any
	// library (TMDb). Without it, series are only known after import.
	Lookup SeriesLookup

	// PollInterval is how often AfterIngest looks for a newly ingested movie.
	PollInterval time.Duration
	// RescanInterval is how often AfterIngest re-triggers a library scan
	// while the movie still hasn't shown up.
	RescanInterval time.Duration
}

func NewSyncer(libs ...Library) *Syncer {
	return &Syncer{
		libs:           libs,
		PollInterval:   15 * time.Second,
		RescanInterval: 5 * time.Minute,
	}
}

// Enabled reports whether any library is configured.
func (s *Syncer) Enabled() bool {
	return s != nil && len(s.libs) > 0
}

// AllCollections returns the union of collections across all libraries,
// de-duplicated case-insensitively and sorted by name. A collection counts
// as a series if any library says it is.
func (s *Syncer) AllCollections(ctx context.Context) ([]Collection, error) {
	all := make([]Collection, 0)
	var errs []error
	for _, lib := range s.libs {
		cols, err := lib.Collections(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lib.Name(), err))
			continue
		}
		for _, c := range cols {
			c.Name = strings.TrimSpace(c.Name)
			if c.Name == "" {
				continue
			}
			i := slices.IndexFunc(all, func(a Collection) bool { return strings.EqualFold(a.Name, c.Name) })
			if i < 0 {
				all = append(all, c)
			} else {
				all[i].Series = all[i].Series || c.Series
				if all[i].SeriesId == "" {
					all[i].SeriesId = c.SeriesId
				}
			}
		}
	}
	slices.SortFunc(all, func(a, b Collection) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return all, errors.Join(errs...)
}

// Suggestion is the series a movie belongs to and the existing collections
// for it.
type Suggestion struct {
	Series Series
	// Matches are the existing collections for the series. Usually one;
	// several when a series has e.g. regular and extended edition sets, and
	// none when nobody has made a collection for it yet.
	Matches []string
}

// SuggestSeries identifies the movie's series and which of all (from
// AllCollections) belong to it. Returns nil if the movie isn't in a series.
func (s *Syncer) SuggestSeries(ctx context.Context, q MovieQuery, all []Collection) (*Suggestion, error) {
	series, err := s.identifySeries(ctx, q)
	if series == nil {
		return nil, err
	}
	sugg := &Suggestion{Series: *series}
	for _, c := range all {
		if c.SeriesId != "" && c.SeriesId == series.TmdbId {
			sugg.Matches = append(sugg.Matches, c.Name)
		}
	}
	return sugg, nil
}

// identifySeries asks the Lookup (TMDb) first, falling back to a library
// that already has the movie.
func (s *Syncer) identifySeries(ctx context.Context, q MovieQuery) (*Series, error) {
	var errs []error
	if s.Lookup != nil && q.ImdbId != "" {
		series, err := s.Lookup.SeriesForMovie(ctx, q.ImdbId)
		if err == nil {
			return series, nil
		}
		errs = append(errs, err)
	}
	for _, lib := range s.libs {
		id, err := lib.FindMovie(ctx, q)
		if errors.Is(err, ErrNotFound) {
			continue
		} else if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lib.Name(), err))
			continue
		}
		seriesId, err := lib.MovieSeriesId(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lib.Name(), err))
			continue
		}
		if seriesId != "" {
			return &Series{TmdbId: seriesId}, nil
		}
	}
	return nil, errors.Join(errs...)
}

// Membership returns the collections the movie currently belongs to across
// all libraries it was found in. found is false if no library knows it.
func (s *Syncer) Membership(ctx context.Context, q MovieQuery) (names []string, found bool, err error) {
	names = make([]string, 0)
	var errs []error
	for _, lib := range s.libs {
		id, err := lib.FindMovie(ctx, q)
		if errors.Is(err, ErrNotFound) {
			continue
		} else if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lib.Name(), err))
			continue
		}
		found = true
		cols, err := lib.MovieCollections(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lib.Name(), err))
			continue
		}
		names = append(names, cols...)
	}
	return SortCollections(NormalizeCollections(names)), found, errors.Join(errs...)
}

// Apply adds the movie to every collection in want. When remove is true it
// also removes the movie from any collection not in want, so only pass true
// when want was chosen starting from the movie's live membership. Returns
// ErrNotFound if no library knows the movie.
func (s *Syncer) Apply(ctx context.Context, q MovieQuery, want []string, remove bool) error {
	found := false
	var errs []error
	for _, lib := range s.libs {
		id, err := lib.FindMovie(ctx, q)
		if errors.Is(err, ErrNotFound) {
			continue
		} else if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lib.Name(), err))
			continue
		}
		found = true
		if err := applyToMovie(ctx, lib, id, want, remove); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lib.Name(), err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func applyToMovie(ctx context.Context, lib Library, movieId string, want []string, remove bool) error {
	current, err := lib.MovieCollections(ctx, movieId)
	if err != nil {
		return err
	}
	add, del := diffCollections(current, want)
	var errs []error
	for _, name := range add {
		log.Println(lib.Name(), "adding", movieId, "to collection", name)
		if err := lib.AddToCollection(ctx, movieId, name); err != nil {
			errs = append(errs, err)
		}
	}
	if remove {
		for _, name := range del {
			log.Println(lib.Name(), "removing", movieId, "from collection", name)
			if err := lib.RemoveFromCollection(ctx, movieId, name); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// AfterIngest triggers a scan in every library, waits for the movie to
// appear, and adds it to the collections in want. A nil want means nobody
// chose collections, so the movie is added to its series' collection instead
// (only when exactly one exists; it never creates one). An empty, non-nil
// want means none were chosen on purpose. It returns when every library is
// done or ctx expires, so give ctx a deadline.
func (s *Syncer) AfterIngest(ctx context.Context, q MovieQuery, want []string) error {
	if want != nil {
		want = NormalizeCollections(want)
	}
	var wg sync.WaitGroup
	errs := make([]error, len(s.libs))
	for i, lib := range s.libs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.afterIngest(ctx, lib, q, want); err != nil {
				errs[i] = fmt.Errorf("%s: %w", lib.Name(), err)
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (s *Syncer) afterIngest(ctx context.Context, lib Library, q MovieQuery, want []string) error {
	if err := lib.Scan(ctx); err != nil {
		log.Println(lib.Name(), "error triggering scan:", err)
	}
	if want != nil && len(want) == 0 {
		return nil
	}

	lastScan := time.Now()
	for {
		id, err := lib.FindMovie(ctx, q)
		if err == nil {
			if want == nil {
				if want, err = s.autoSeries(ctx, lib, q, id); err != nil {
					return err
				}
			}
			return applyToMovie(ctx, lib, id, want, false)
		}
		if !errors.Is(err, ErrNotFound) {
			log.Println(lib.Name(), "error finding movie", q, err)
		}

		if time.Since(lastScan) >= s.RescanInterval {
			if err := lib.Scan(ctx); err != nil {
				log.Println(lib.Name(), "error triggering scan:", err)
			}
			lastScan = time.Now()
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("gave up waiting for %v: %w", q, ctx.Err())
		case <-time.After(s.PollInterval):
		}
	}
}

// autoSeries picks the collection to add a newly ingested movie to when none
// were chosen: its series' collection, if exactly one exists in lib.
func (s *Syncer) autoSeries(ctx context.Context, lib Library, q MovieQuery, movieId string) ([]string, error) {
	seriesId, err := lib.MovieSeriesId(ctx, movieId)
	if err != nil {
		return nil, err
	}
	if seriesId == "" && s.Lookup != nil {
		if series, err := s.Lookup.SeriesForMovie(ctx, q.ImdbId); err != nil {
			log.Println("error looking up series for", q, err)
		} else if series != nil {
			seriesId = series.TmdbId
		}
	}
	if seriesId == "" {
		return []string{}, nil
	}

	cols, err := lib.Collections(ctx)
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0)
	for _, c := range cols {
		if c.SeriesId == seriesId {
			matches = append(matches, c.Name)
		}
	}
	if len(matches) != 1 {
		log.Println(lib.Name(), "not auto-adding", q, "to series", seriesId, "- matching collections:", matches)
		return []string{}, nil
	}
	return matches, nil
}

// diffCollections returns which names in want are missing from current, and
// which names in current aren't in want, comparing case-insensitively.
func diffCollections(current, want []string) (add, del []string) {
	for _, w := range want {
		if !ContainsCollection(current, w) {
			add = append(add, w)
		}
	}
	for _, c := range current {
		if !ContainsCollection(want, c) {
			del = append(del, c)
		}
	}
	return add, del
}

// NormalizeCollections trims names, drops empty ones, and removes
// case-insensitive duplicates (keeping the first spelling seen).
func NormalizeCollections(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" && !ContainsCollection(out, n) {
			out = append(out, n)
		}
	}
	return out
}

func SortCollections(names []string) []string {
	slices.SortFunc(names, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	return names
}

func ContainsCollection(names []string, name string) bool {
	return slices.ContainsFunc(names, func(n string) bool {
		return strings.EqualFold(n, name)
	})
}
