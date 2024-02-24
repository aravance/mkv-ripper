package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
	workflowview "github.com/aravance/mkv-ripper/view/workflow"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
)

type WorkflowHandler struct {
	wfman    workflow.WorkflowManager
	driveman drive.DriveManager
	discdb   drive.DiscDatabase
	omdbapi  *gomdb.OmdbApi
}

func NewWorkflowHandler(
	wfman workflow.WorkflowManager,
	driveman drive.DriveManager,
	discdb drive.DiscDatabase,
	omdbapi *gomdb.OmdbApi,
) WorkflowHandler {
	return WorkflowHandler{
		wfman:    wfman,
		driveman: driveman,
		discdb:   discdb,
		omdbapi:  omdbapi,
	}
}

func (h WorkflowHandler) GetWorkflow(c echo.Context) error {
	var w *model.Workflow
	var d *drive.Disc
	var m *gomdb.MovieResult

	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}

	w = h.wfman.GetWorkflow(discId, titleId)
	if w == nil {
		di, ok := h.discdb.GetDiscInfo(discId)
		if !ok {
			return c.NoContent(http.StatusNotFound)
		}
		ti := &di.Titles[titleId]
		name := util.GuessName(di, ti)

		w, _ = h.wfman.NewWorkflow(discId, titleId, di.VolumeName, name)
		h.wfman.Save(w)
	}

	d = h.driveman.GetDisc()
	if d != nil && d.Uuid != discId {
		d = nil
	}

	if w.ImdbId != nil {
		m, err = h.omdbapi.MovieByImdbID(*w.ImdbId)
	} else {
		name := w.Name
		if name == nil {
			name = &w.OriginalName
		}
		m, err = util.GetMovie(*name, h.omdbapi)
		if err != nil {
			log.Println("error getting movie:", *name, "err:", err)
			m = nil
		}
	}

	if w == nil && d == nil {
		return c.NoContent(http.StatusNotFound)
	}
	return render(c, workflowview.Show(w, d, m))
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

func (h WorkflowHandler) Status(c echo.Context) error {
	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	var w *model.Workflow
	if err == nil {
		w = h.wfman.GetWorkflow(discId, titleId)
	}
	if w == nil {
		return c.NoContent(http.StatusNotFound)
	}
	return render(c, workflowview.Status(w))
}

func (h WorkflowHandler) RipTitle(c echo.Context) error {
	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	status := h.driveman.Status()
	if err != nil {
		return c.String(http.StatusNotFound, "no title found")
	}
	if status == drive.StatusEmpty {
		return c.String(http.StatusNotFound, "drive is empty")
	}
	if status != drive.StatusReady {
		return c.String(http.StatusNotFound, "drive is busy")
	}
	disc := h.driveman.GetDisc()
	if disc.Uuid != discId {
		return c.String(http.StatusNotFound, "disc changed")
	}
	discInfo, ok := h.discdb.GetDiscInfo(discId)
	if !ok {
		return c.String(http.StatusNotFound, "disc info not found")
	}
	titleInfo := discInfo.Titles[titleId]
	name := util.GuessName(discInfo, &titleInfo)

	wf, ok := h.wfman.NewWorkflow(discId, titleId, disc.Label, name)

	if wf.Name == nil || *wf.Name == "" || wf.Year == nil || *wf.Year == "" {
		if movie, err := util.GetMovie(wf.OriginalName, h.omdbapi); err != nil {
			log.Println("failed to GetMovie", err)
		} else {
			wf.Name = &movie.Title
			wf.Year = &movie.Year
			wf.ImdbId = &movie.ImdbID
			h.wfman.Save(wf)
		}
	}

	go h.wfman.Start(wf)
	return c.Redirect(http.StatusSeeOther, util.WorkflowUrl(wf.DiscId, wf.TitleId))
}
