package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/aravance/mkv-ripper/model"
	workflowview "github.com/aravance/mkv-ripper/view/workflow"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
)

type WorkflowHandler struct {
	wfman   workflow.WorkflowManager
	omdbapi *gomdb.OmdbApi
}

func NewWorkflowHandler(wfman workflow.WorkflowManager, omdbapi *gomdb.OmdbApi) WorkflowHandler {
	return WorkflowHandler{
		wfman:   wfman,
		omdbapi: omdbapi,
	}
}

func (h WorkflowHandler) GetWorkflow(c echo.Context) error {
	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	var w *model.Workflow
	if err == nil {
		w = h.wfman.GetWorkflow(discId, titleId)
	}
	if w == nil {
		return c.NoContent(http.StatusNotFound)
	}
	return render(c, workflowview.Show(w))
}

func (h WorkflowHandler) EditWorkflow(c echo.Context) error {
	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	log.Println("getting workflow", discId, ":", titleId)
	var w *model.Workflow
	if err == nil {
		w = h.wfman.GetWorkflow(discId, titleId)
	}
	log.Println("got workflow", w)
	if w == nil {
		return c.NoContent(http.StatusNotFound)
	}
	return render(c, workflowview.Edit(w))
}

func (h WorkflowHandler) PostWorkflow(c echo.Context) error {
	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	var w *model.Workflow
	if err == nil {
		w = h.wfman.GetWorkflow(discId, titleId)
	}
	if w == nil {
		return c.NoContent(http.StatusNotFound)
	}

	imdbid := c.FormValue("imdbid")
	if imdbid == "" {
		return c.String(http.StatusUnprocessableEntity, "imdbid cannot be empty")
	}

	mov, err := h.omdbapi.MovieByImdbID(imdbid)
	if err != nil {
		log.Println("error fetching movie", imdbid, "err:", err)
		return c.String(http.StatusInternalServerError, fmt.Sprintf("error fetching movie, %v", err))
	}

	w.Name = &mov.Title
	w.Year = &mov.Year
	w.ImdbId = &imdbid
	if err := h.wfman.Save(w); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
	}

	if w.File != nil {
		go h.wfman.Ingest(w)
	}
	return c.Redirect(http.StatusSeeOther, "/")
}
