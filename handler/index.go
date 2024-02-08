package handler

import (
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
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
	all := i.workflowManager.GetAllWorkflows()
	workflows := make([]*model.Workflow, 0)
	for _, wf := range all {
		if wf.Status == model.StatusRipping || wf.Status == model.StatusImporting || wf.Status == model.StatusPending {
			workflows = append(workflows, wf)
		}
	}
	drivestat := i.driveManager.Status()
	return render(c, indexview.Show(drivestat, workflows))
}
