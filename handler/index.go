package handler

import (
	"slices"
	"strings"

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
	active := make([]*model.Workflow, 0)
	errored := make([]*model.Workflow, 0)
	done := make([]*model.Workflow, 0)
	for _, wf := range all {
		switch wf.Status {
		case model.StatusPending:
			fallthrough
		case model.StatusImporting:
			fallthrough
		case model.StatusRipping:
			active = append(active, wf)

		case model.StatusError:
			errored = append(errored, wf)

		case model.StatusDone:
			done = append(done, wf)
		}
	}

	slices.SortFunc(active, compareWorkflows)
	slices.SortFunc(errored, compareWorkflows)
	slices.SortFunc(done, compareWorkflows)

	drstatus := i.driveManager.Status()
	return render(c, indexview.Show(drstatus, active, errored, done))
}

func normalizeTitle(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	t, _ = strings.CutPrefix(t, "a ")
	t, _ = strings.CutPrefix(t, "the ")
	return t
}

func compareWorkflows(a, b *model.Workflow) int {
	if a == nil {
		if b == nil {
			return 0
		} else {
			return -1
		}
	}
	if b == nil {
		return 1
	}

	if a.Name == nil {
		if b.Name == nil {
			return 0
		} else {
			return -1
		}
	}
	if b.Name == nil {
		return 1
	}

	comp := strings.Compare(normalizeTitle(*a.Name), normalizeTitle(*b.Name))
	if comp != 0 {
		return comp
	}
	return strings.Compare(a.DiscId, b.DiscId)
}
