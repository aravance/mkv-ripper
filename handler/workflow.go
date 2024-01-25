package handler

import (
	"fmt"
	"net/http"

	"github.com/aravance/mkv-ripper/model"
	workflowview "github.com/aravance/mkv-ripper/view/workflow"
	"github.com/labstack/echo/v4"
)

type WorkflowHandler struct {
	workflowManager model.WorkflowManager
	inchan          chan *model.Workflow
}

func NewWorkflowHandler(workflowManager model.WorkflowManager, inchan chan *model.Workflow) WorkflowHandler {
	return WorkflowHandler{workflowManager, inchan}
}

func (h WorkflowHandler) GetWorkflow(c echo.Context) error {
	id := c.Param("id")
	w := h.workflowManager.GetWorkflow(id)
	if w == nil {
		return c.NoContent(http.StatusNotFound)
	}
	return render(c, workflowview.Show(w))
}

func (h WorkflowHandler) PostWorkflow(c echo.Context) error {
	id := c.Param("id")
	w := h.workflowManager.GetWorkflow(id)
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
		go func(w *model.Workflow) {
			h.inchan <- w
		}(w)
		return c.String(http.StatusOK, "Import started")
	} else {
		return c.String(http.StatusOK, "Import will begin once the files are ready")
	}
}