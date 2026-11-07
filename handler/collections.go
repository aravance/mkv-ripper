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
	selected := storedCollections(wfs)
	live := false
	if anyDone(wfs) {
		names, found, err := h.syncer.Membership(ctx, library.QueryForWorkflow(wfs[0]))
		if err != nil {
			log.Println("error getting collection membership:", err)
			msg = joinMessages(msg, "Couldn't load current collections: "+err.Error())
		} else if found {
			selected = names
			live = true
		}
	}

	return render(c, collectionview.Form(action, collectionOptions(all, selected), selected, live, msg))
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

// storedCollections is the union of the collections saved on wfs.
func storedCollections(wfs []*model.Workflow) []string {
	names := make([]string, 0)
	for _, wf := range wfs {
		names = append(names, wf.Collections...)
	}
	return library.SortCollections(library.NormalizeCollections(names))
}

// collectionOptions lists every collection the picker should offer: all the
// server's collections plus any selected ones that don't exist there yet.
func collectionOptions(all, selected []string) []string {
	names := append(slices.Clone(all), selected...)
	return library.SortCollections(library.NormalizeCollections(names))
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
