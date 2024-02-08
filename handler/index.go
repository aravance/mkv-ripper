package handler

import (
	"github.com/aravance/mkv-ripper/drive"
	indexview "github.com/aravance/mkv-ripper/view/index"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/labstack/echo/v4"
)

type IndexHandler struct {
	workflowManager workflow.WorkflowManager
	driveManager    drive.DriveManager
}

func NewIndexHandler(driveManager drive.DriveManager, workflowManager workflow.WorkflowManager) IndexHandler {
	return IndexHandler{workflowManager, driveManager}
}

func (i IndexHandler) GetIndex(c echo.Context) error {
	workflows := i.workflowManager.GetWorkflows()
	drivestat := i.driveManager.Status()
	return render(c, indexview.Show(drivestat, workflows))
}
