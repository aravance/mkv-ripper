package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/aravance/mkv-ripper/model"
	workflowview "github.com/aravance/mkv-ripper/view/workflow"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/labstack/echo/v4"
)

type WorkflowHandler struct {
	workflowManager workflow.WorkflowManager
}

func NewWorkflowHandler(workflowManager workflow.WorkflowManager) WorkflowHandler {
	return WorkflowHandler{
		workflowManager: workflowManager,
	}
}

func (h WorkflowHandler) GetWorkflow(c echo.Context) error {
	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	var w *model.Workflow
	if err == nil {
		w = h.workflowManager.GetWorkflow(discId, titleId)
	}
	if w == nil {
		return c.NoContent(http.StatusNotFound)
	}
	return render(c, workflowview.Show(w))
}

func (h WorkflowHandler) EditWorkflow(c echo.Context) error {
	discId := c.Param("discId")
	titleId, err := strconv.Atoi(c.Param("titleId"))
	var w *model.Workflow
	if err == nil {
		w = h.workflowManager.GetWorkflow(discId, titleId)
	}
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
		w = h.workflowManager.GetWorkflow(discId, titleId)
	}
	if w == nil {
		return c.NoContent(http.StatusNotFound)
	}

	if w.Name != nil || w.Year != nil {
		return c.String(http.StatusConflict, "workflow already has details")
	}

	name := c.FormValue("name")
	if name == "" {
		return c.String(http.StatusUnprocessableEntity, "name cannot be empty")
	}
	year := c.FormValue("year")
	if year == "" {
		return c.String(http.StatusUnprocessableEntity, "year cannot be empty")
	}

	w.Name = &name
	w.Year = &year
	if err := h.workflowManager.Save(w); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
	}

	if w.File != nil {
		go h.workflowManager.Ingest(w)
		return c.String(http.StatusOK, "Import started")
	} else {
		return c.String(http.StatusOK, "Import will begin once the files are ready")
	}
}
