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

// AllCollections returns the sorted, case-insensitively de-duplicated union
// of collection names across all libraries.
func (s *Syncer) AllCollections(ctx context.Context) ([]string, error) {
	all := make([]string, 0)
	var errs []error
	for _, lib := range s.libs {
		names, err := lib.Collections(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", lib.Name(), err))
			continue
		}
		all = append(all, names...)
	}
	return SortCollections(NormalizeCollections(all)), errors.Join(errs...)
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

// AfterIngest triggers a scan in every library and, if want is non-empty,
// waits for the movie to appear and adds it to those collections. It returns
// when every library is done or ctx expires, so give ctx a deadline.
func (s *Syncer) AfterIngest(ctx context.Context, q MovieQuery, want []string) error {
	want = NormalizeCollections(want)
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
	if len(want) == 0 {
		return nil
	}

	lastScan := time.Now()
	for {
		id, err := lib.FindMovie(ctx, q)
		if err == nil {
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
