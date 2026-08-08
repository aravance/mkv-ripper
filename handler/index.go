package handler

import (
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
	indexview "github.com/aravance/mkv-ripper/view/index"
	movieview "github.com/aravance/mkv-ripper/view/movie"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
)

const recentMovieLimit = 5

type IndexHandler struct {
	workflowManager workflow.WorkflowManager
	driveManager    drive.DriveManager
	omdbapi         *gomdb.OmdbApi
	posters         *posterCache
}

func NewIndexHandler(driveManager drive.DriveManager, workflowManager workflow.WorkflowManager, omdbapi *gomdb.OmdbApi) IndexHandler {
	return IndexHandler{workflowManager, driveManager, omdbapi, newPosterCache(omdbapi)}
}

func (i IndexHandler) GetIndex(c echo.Context) error {
	active, errored, done := i.splitWorkflows()

	slices.SortFunc(active, compareWorkflows)
	slices.SortFunc(errored, compareWorkflows)

	movies := groupMovies(done)
	recent := movies
	if len(recent) > recentMovieLimit {
		recent = recent[:recentMovieLimit]
	}
	i.posters.fill(recent)

	drstatus := i.driveManager.Status()
	return render(c, indexview.Show(drstatus, active, errored, recent, len(movies) > len(recent)))
}

func (i IndexHandler) GetHistory(c echo.Context) error {
	_, _, done := i.splitWorkflows()
	movies := groupMovies(done)
	slices.SortFunc(movies, compareMovieGroups)
	i.posters.fill(movies)
	return render(c, indexview.History(movies))
}

func (i IndexHandler) GetMovie(c echo.Context) error {
	imdbId := c.Param("imdbId")
	if imdbId == "" {
		return c.NoContent(http.StatusNotFound)
	}

	_, _, done := i.splitWorkflows()
	done = slices.DeleteFunc(done, func(wf *model.Workflow) bool {
		return wf.ImdbId == nil || *wf.ImdbId != imdbId
	})
	if len(done) == 0 {
		return c.NoContent(http.StatusNotFound)
	}

	groups := groupMovies(done)
	group := groups[0]

	movie, err := i.omdbapi.MovieByImdbID(imdbId)
	if err != nil {
		log.Println("error fetching movie", imdbId, "err:", err)
		movie = nil
	}

	return render(c, movieview.Page(movie, group))
}

// splitWorkflows buckets all workflows by status, keeping only the
// highest title id per disc among done workflows (a disc may have several
// ripped titles, e.g. extras, but only its main feature counts as history).
func (i IndexHandler) splitWorkflows() (active, errored, done []*model.Workflow) {
	all := i.workflowManager.GetAllWorkflows()
	active = make([]*model.Workflow, 0)
	errored = make([]*model.Workflow, 0)
	done = make([]*model.Workflow, 0)
	for _, wf := range all {
		switch wf.Status {
		case model.StatusPending:
			fallthrough
		case model.StatusImporting:
			fallthrough
		case model.StatusRipping:
			active = append(active, wf)

		case model.StatusError:
			errored = append(errored, wf)

		case model.StatusDone:
			done = append(done, wf)
		}
	}

	done = slices.DeleteFunc(done, func(wf *model.Workflow) bool {
		return slices.ContainsFunc(done, func(o *model.Workflow) bool {
			return o.DiscId == wf.DiscId && o.TitleId > wf.TitleId
		})
	})

	errored = slices.DeleteFunc(errored, func(wf *model.Workflow) bool {
		matches := func(o *model.Workflow) bool {
			if wf == nil {
				return o == nil
			}
			return wf.ImdbId != nil && o.ImdbId != nil && *wf.ImdbId == *o.ImdbId
		}
		return slices.ContainsFunc(done, matches) || slices.ContainsFunc(active, matches)
	})

	return active, errored, done
}

func normalizeTitle(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	t, _ = strings.CutPrefix(t, "a ")
	t, _ = strings.CutPrefix(t, "the ")
	return t
}

func compareWorkflows(a, b *model.Workflow) int {
	if a == nil {
		if b == nil {
			return 0
		} else {
			return -1
		}
	}
	if b == nil {
		return 1
	}

	if a.Name == nil {
		if b.Name == nil {
			return 0
		} else {
			return -1
		}
	}
	if b.Name == nil {
		return 1
	}

	comp := strings.Compare(normalizeTitle(*a.Name), normalizeTitle(*b.Name))
	if comp != 0 {
		return comp
	}
	if a.DiscId != b.DiscId {
		return strings.Compare(a.DiscId, b.DiscId)
	}
	return b.TitleId - a.TitleId
}

func compareMovieGroups(a, b *model.MovieGroup) int {
	return strings.Compare(normalizeTitle(a.Name), normalizeTitle(b.Name))
}
