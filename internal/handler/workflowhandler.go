package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/aravance/mkv-ripper/internal/model"
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
	workflow, err := model.LoadWorkflow(id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.NoContent(http.StatusNotFound)
		} else {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
		}
	}
	if bytes, err := json.Marshal(workflow); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
	} else {
		return c.JSONBlob(http.StatusOK, bytes)
	}
}

func (h WorkflowHandler) PostWorkflow(c echo.Context) error {
	id := c.Param("id")
	workflow, err := model.LoadWorkflow(id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c.NoContent(http.StatusNotFound)
		} else {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
		}
	}

	if workflow.Name != nil || workflow.Year != nil {
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

	workflow.Name = &name
	workflow.Year = &year
	workflow.Save()

	go func(w *model.Workflow) {
		h.inchan <- w
	}(workflow)

	if bytes, err := json.Marshal(workflow); err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("%v", err))
	} else {
		return c.JSONBlob(http.StatusOK, bytes)
	}
}

