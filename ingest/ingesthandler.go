package ingest

import (
	"log"
	"net/url"
	"os"
	"path"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
	"github.com/eefret/gomdb"
)

type IngestHandler struct {
	discdb          drive.DiscDatabase
	driveManager    drive.DriveManager
	workflowManager model.WorkflowManager
	omdbapi         *gomdb.OmdbApi
	targets         []*url.URL
	outdir          string
}

func (h *IngestHandler) Init(discdb drive.DiscDatabase, driveManager drive.DriveManager, workflowManager model.WorkflowManager, omdbapi *gomdb.OmdbApi, targets []*url.URL, outdir string) {
	h.discdb = discdb
	h.driveManager = driveManager
	h.workflowManager = workflowManager
	h.omdbapi = omdbapi
	h.targets = targets
	h.outdir = outdir
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

func (h *IngestHandler) HandleDisc(disc *drive.Disc) {
	if disc == nil {
		return
	}

	var info *makemkv.DiscInfo
	var found bool
	var err error
	info, found = h.discdb.GetDiscInfo(disc.Uuid)
	if !found {
		info, err = h.driveManager.GetDiscInfo()
		if err != nil {
			log.Println("error getting disc info:", disc, "err:", err)
		}
		err = h.discdb.SaveDiscInfo(disc.Uuid, info)
		if err != nil {
			log.Println("error saving disc info:", disc, "err:", err)
		}

		main := util.GuessMainTitle(info)
		movie, err := util.GetMovie(h.omdbapi, main.Name)
		if err != nil {
			return
		}

		wf, _ := h.workflowManager.NewWorkflow(disc.Uuid, disc.Label)
		wf.Label = disc.Label
		wf.Name = &movie.Title
		wf.Year = &movie.Year
		wf.Status = model.StatusRipping
		h.workflowManager.Save(wf)

		dir := path.Join(h.outdir, wf.Id)
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			log.Println("error making dir:", dir, "err:", err)
			return
		}

		statchan := make(chan makemkv.Status)
		defer close(statchan)

		go func() {
			for stat := range statchan {
				wf.MkvStatus = &stat
			}
		}()

		f, err := h.driveManager.RipFile(main, dir, statchan)
		if err != nil {
			log.Println("error ripping:", wf, "err:", err)
			return
		}

		wf.File = f
		wf.Status = model.StatusPending
		h.workflowManager.Save(wf)

		go h.IngestWorkflow(wf)
	}
}
