package handler

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/view/workflow"
	"github.com/labstack/echo/v4"
)

type WorkflowHandler struct {
	inchan chan *model.Workflow
}

func NewWorkflowHandler(inchan chan *model.Workflow) WorkflowHandler {
	return WorkflowHandler{inchan}
}

func (h WorkflowHandler) GetWorkflow(c echo.Context) error {
	id := c.Param("id")
	w, err := model.LoadWorkflow(id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.NoContent(http.StatusNotFound)
		} else {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
		}
	}
	return render(c, workflow.Show(w))
}

func (h WorkflowHandler) PostWorkflow(c echo.Context) error {
	id := c.Param("id")
	w, err := model.LoadWorkflow(id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.NoContent(http.StatusNotFound)
		} else {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
		}
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

	w.AddMovieDetails(name, year)
	if err := w.Save(); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
	}

	if len(w.Files) > 0 {
		go func(w *model.Workflow) {
			h.inchan <- w
		}(w)
		return c.String(http.StatusOK, "Import started")
	} else {
		return c.String(http.StatusOK, "Import will begin once the files are ready")
	}
}
