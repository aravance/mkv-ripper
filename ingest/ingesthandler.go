package ingest

import (
	"log"
	"net/url"

	"github.com/aravance/mkv-ripper/model"
)

type IngestHandler struct {
	workflowManager model.WorkflowManager
	targets         []*url.URL
}

func NewIngestHandler(workflowManager model.WorkflowManager, targets []*url.URL) *IngestHandler {
	return &IngestHandler{
		workflowManager: workflowManager,
		targets:         targets,
	}
}

func (h *IngestHandler) IngestWorkflow(workflow *model.Workflow) {
	log.Println("ingesting", workflow)

	mkv := workflow.File
	if mkv == nil {
		log.Println("no files to ingest")
		return
	}

	workflow.Status = model.StatusImporting
	h.workflowManager.Save(workflow)

	var err error
	for _, target := range h.targets {
		ingester, err := NewIngester(target)
		if err != nil {
			log.Println("error finding ingester", err, "for target", target)
			continue
		}

		err = ingester.Ingest(*mkv, *workflow.Name, *workflow.Year)
		if err != nil {
			log.Println("error running ingester", ingester, err)
		}
	}

	if err == nil {
		log.Println("cleaning workflow")
		h.workflowManager.Clean(workflow)
		workflow.Status = model.StatusDone
		h.workflowManager.Save(workflow)
	}
}
