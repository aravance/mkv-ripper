package ingest

import (
	"fmt"
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

func (h *IngestHandler) RipTitle(wf *model.Workflow, disc *drive.Disc, di *makemkv.DiscInfo, ti *makemkv.TitleInfo) error {
	driveStatus := h.driveManager.Status()
	if driveStatus != drive.StatusReady {
		log.Println("disc is not ready:", driveStatus)
		return fmt.Errorf("disc is not ready: %s", driveStatus)
	}
	if wf == nil {
		return fmt.Errorf("wf cannot be nil")
	}
	if disc == nil {
		return fmt.Errorf("disc cannot be nil")
	}
	if di == nil {
		return fmt.Errorf("info cannot be nil")
	}
	if ti == nil {
		return fmt.Errorf("title cannot be nil")
	}

	wf.Status = model.StatusRipping
	h.workflowManager.Save(wf)

	if movie, err := util.GetMovie(h.omdbapi, wf.OriginalName); err != nil {
		log.Println("failed to GetMovie", err)
	} else {
		wf.Name = &movie.Title
		wf.Year = &movie.Year
		wf.ImdbId = &movie.ImdbID
		h.workflowManager.Save(wf)
	}

	dir := path.Join(h.outdir, wf.Id())
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Println("error making dir:", dir, "err:", err)
		wf.Status = model.StatusError
		h.workflowManager.Save(wf)
		return err
	}

	statchan := make(chan makemkv.Status)
	defer close(statchan)

	go func() {
		for stat := range statchan {
			disc.MkvStatus = &stat
		}
	}()

	f, err := h.driveManager.RipFile(ti, dir, statchan)
	if err != nil {
		log.Println("error ripping:", wf, "err:", err)
		wf.Status = model.StatusError
		h.workflowManager.Save(wf)
		return err
	}

	wf.File = f
	wf.Status = model.StatusPending
	h.workflowManager.Save(wf)
	return nil
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
			return
		}
		err = h.discdb.SaveDiscInfo(disc.Uuid, info)
		if err != nil {
			log.Println("error saving disc info:", disc, "err:", err)
			return
		}

		main := util.GuessMainTitle(info)
		if main == nil {
			log.Println("failed to guess main title")
			return
		}

		name := util.GuessName(info, main)
		wf, _ := h.workflowManager.NewWorkflow(disc.Uuid, main.Id, disc.Label, name)
		err := h.RipTitle(wf, disc, info, main)
		if err != nil {
			log.Println("failed to rip title", err)
			return
		}

		go h.IngestWorkflow(wf)
	}
}
