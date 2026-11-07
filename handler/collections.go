package handler

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aravance/mkv-ripper/library"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
	collectionview "github.com/aravance/mkv-ripper/view/collection"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/labstack/echo/v4"
)

const libraryTimeout = 30 * time.Second

// CollectionsHandler serves the collections picker shown on the workflow and
// movie pages. Choices are saved on the workflow(s) so they can be applied
// after ingest; for movies that are already done they're applied to the
// media server(s) immediately.
type CollectionsHandler struct {
	wfman  workflow.WorkflowManager
	syncer *library.Syncer
}

func NewCollectionsHandler(wfman workflow.WorkflowManager, syncer *library.Syncer) CollectionsHandler {
	return CollectionsHandler{wfman, syncer}
}

func (h CollectionsHandler) workflow(c echo.Context) *model.Workflow {
	titleId, err := strconv.Atoi(c.Param("titleId"))
	if err != nil {
		return nil
	}
	return h.wfman.GetWorkflow(c.Param("discId"), titleId)
}

// movieWorkflows returns the done workflows for a movie, i.e. the rips that
// make up its movie page.
func (h CollectionsHandler) movieWorkflows(imdbId string) []*model.Workflow {
	return slices.DeleteFunc(h.wfman.GetAllWorkflows(), func(wf *model.Workflow) bool {
		return wf.Status != model.StatusDone || wf.ImdbId == nil || *wf.ImdbId != imdbId
	})
}

func (h CollectionsHandler) GetWorkflowCollections(c echo.Context) error {
	wf := h.workflow(c)
	if wf == nil {
		return c.NoContent(http.StatusNotFound)
	}
	return h.renderForm(c, util.WorkflowUrl(wf.DiscId, wf.TitleId, "collections"), []*model.Workflow{wf}, "")
}

func (h CollectionsHandler) PostWorkflowCollections(c echo.Context) error {
	wf := h.workflow(c)
	if wf == nil {
		return c.NoContent(http.StatusNotFound)
	}
	return h.save(c, util.WorkflowUrl(wf.DiscId, wf.TitleId, "collections"), []*model.Workflow{wf})
}

func (h CollectionsHandler) GetMovieCollections(c echo.Context) error {
	imdbId := c.Param("imdbId")
	wfs := h.movieWorkflows(imdbId)
	if len(wfs) == 0 {
		return c.NoContent(http.StatusNotFound)
	}
	return h.renderForm(c, util.MovieUrl(imdbId, "collections"), wfs, "")
}

func (h CollectionsHandler) PostMovieCollections(c echo.Context) error {
	imdbId := c.Param("imdbId")
	wfs := h.movieWorkflows(imdbId)
	if len(wfs) == 0 {
		return c.NoContent(http.StatusNotFound)
	}
	return h.save(c, util.MovieUrl(imdbId, "collections"), wfs)
}

func (h CollectionsHandler) renderForm(c echo.Context, action string, wfs []*model.Workflow, msg string) error {
	if !h.syncer.Enabled() {
		return c.HTML(http.StatusOK, "")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), libraryTimeout)
	defer cancel()

	all, err := h.syncer.AllCollections(ctx)
	if err != nil {
		log.Println("error listing collections:", err)
		msg = joinMessages(msg, "Couldn't load all collections: "+err.Error())
	}

	// Once the movie is in the library, show what it's actually in (it may
	// have been changed on the server directly); until then, show the saved
	// choices. live tells the POST it's safe to remove unchecked collections.
	q := library.QueryForWorkflow(wfs[0])
	selected := storedCollections(wfs)
	live := false
	if anyDone(wfs) {
		names, found, err := h.syncer.Membership(ctx, q)
		if err != nil {
			log.Println("error getting collection membership:", err)
			msg = joinMessages(msg, "Couldn't load current collections: "+err.Error())
		} else if found {
			selected = names
			live = true
		}
	}

	sugg, err := h.syncer.SuggestSeries(ctx, q, all)
	if err != nil {
		log.Println("error identifying series for", q, err)
	}

	p := buildPicker(all, selected, neverChosen(wfs), sugg)
	p.Action = action
	p.Live = live
	p.Msg = msg
	return render(c, collectionview.Form(p))
}

func (h CollectionsHandler) save(c echo.Context, action string, wfs []*model.Workflow) error {
	form, err := c.FormParams()
	if err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}
	want := wantedCollections(form["collection"], form.Get("new"))

	for _, wf := range wfs {
		wf.Collections = slices.Clone(want)
		if err := h.wfman.Save(wf); err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
	}

	msg := "Saved. They'll be applied after import."
	if anyDone(wfs) {
		ctx, cancel := context.WithTimeout(c.Request().Context(), libraryTimeout)
		defer cancel()
		err := h.syncer.Apply(ctx, library.QueryForWorkflow(wfs[0]), want, form.Get("live") == "true")
		switch {
		case errors.Is(err, library.ErrNotFound):
			msg = "Saved, but the movie isn't in the library yet. Try again once it's been scanned."
		case err != nil:
			log.Println("error applying collections:", err)
			msg = "Saved, but updating the library failed: " + err.Error()
		default:
			msg = "Saved."
		}
	}
	return h.renderForm(c, action, wfs, msg)
}

func anyDone(wfs []*model.Workflow) bool {
	return slices.ContainsFunc(wfs, func(wf *model.Workflow) bool {
		return wf.Status == model.StatusDone
	})
}

// neverChosen reports whether nobody has saved collections for these
// workflows yet, in which case the movie's series is selected automatically
// (matching what happens after ingest).
func neverChosen(wfs []*model.Workflow) bool {
	return !slices.ContainsFunc(wfs, func(wf *model.Workflow) bool {
		return wf.Collections != nil
	})
}

// buildPicker decides what the collections picker offers and pre-selects.
// selected is what the movie is already in (live) or was saved with.
// When autoSelect is set (nothing chosen yet) and the movie's series has
// exactly one collection, it's pre-checked, since that's the collection the
// movie is added to after ingest anyway. Ambiguous series collections (e.g.
// regular and extended editions) are only marked as suggested, and a series
// with no collection yet is offered as a one-click new collection.
func buildPicker(all []library.Collection, selected []string, autoSelect bool, sugg *library.Suggestion) collectionview.Picker {
	current := slices.Clone(selected)
	selected = slices.Clone(selected)
	var suggested []string
	newSeries := ""
	if sugg != nil {
		// only badge series collections the movie isn't already in
		for _, m := range sugg.Matches {
			if !library.ContainsCollection(selected, m) {
				suggested = append(suggested, m)
			}
		}
		if autoSelect && len(sugg.Matches) == 1 && !library.ContainsCollection(selected, sugg.Matches[0]) {
			selected = append(selected, sugg.Matches[0])
		}
		if len(sugg.Matches) == 0 && sugg.Series.Name != "" && !library.ContainsCollection(selected, sugg.Series.Name) {
			newSeries = sugg.Series.Name
		}
	}
	topical, series := splitCollections(all, selected)
	return collectionview.Picker{
		Topical:   topical,
		Series:    series,
		Selected:  selected,
		Current:   current,
		Suggested: suggested,
		NewSeries: newSeries,
	}
}

// storedCollections is the union of the collections saved on wfs.
func storedCollections(wfs []*model.Workflow) []string {
	names := make([]string, 0)
	for _, wf := range wfs {
		names = append(names, wf.Collections...)
	}
	return library.SortCollections(library.NormalizeCollections(names))
}

// splitCollections lists every collection the picker should offer, split
// into topical collections (shown) and release series (tucked away). Selected
// collections that don't exist on the server yet are new, so topical.
func splitCollections(all []library.Collection, selected []string) (topical, series []string) {
	topical = make([]string, 0)
	series = make([]string, 0)
	for _, c := range all {
		if c.Series {
			series = append(series, c.Name)
		} else {
			topical = append(topical, c.Name)
		}
	}
	for _, s := range selected {
		if !library.ContainsCollection(topical, s) && !library.ContainsCollection(series, s) {
			topical = append(topical, s)
		}
	}
	return library.SortCollections(library.NormalizeCollections(topical)), library.SortCollections(library.NormalizeCollections(series))
}

// wantedCollections combines the checked collections with the comma-separated
// new collection names typed into the form.
func wantedCollections(checked []string, extra string) []string {
	names := append(slices.Clone(checked), strings.Split(extra, ",")...)
	return library.NormalizeCollections(names)
}

func joinMessages(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}
