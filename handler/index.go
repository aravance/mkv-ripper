package handler

import (
	"github.com/aravance/mkv-ripper/model"
	indexview "github.com/aravance/mkv-ripper/view/index"
	"github.com/labstack/echo/v4"
)

type IndexHandler struct {
	workflowManager model.WorkflowManager
}

func NewIndexHandler(workflowManager model.WorkflowManager) IndexHandler {
	return IndexHandler{workflowManager}
}

func (i IndexHandler) GetIndex(c echo.Context) error {
	workflows := i.workflowManager.GetWorkflows()
	return render(c, indexview.Show(workflows))
}
