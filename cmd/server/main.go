package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/handler"
	"github.com/aravance/mkv-ripper/ingest"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const LOG_FILE = "./mkv.log"
const OUT_DIR = "."
const API_KEY = ""

var targets = []string{
	"ssh://plexbot/plex",
	"/mnt/nas/plex",
}

type discHandler struct {
	discdb          drive.DiscDatabase
	driveManager    drive.DriveManager
	workflowManager model.WorkflowManager
	omdbapi         *gomdb.OmdbApi
	ingestchan      chan *model.Workflow
}

func (h *discHandler) init(discdb drive.DiscDatabase, driveManager drive.DriveManager, workflowManager model.WorkflowManager, omdbapi *gomdb.OmdbApi, ingestchan chan *model.Workflow) {
	h.discdb = discdb
	h.driveManager = driveManager
	h.workflowManager = workflowManager
	h.omdbapi = omdbapi
	h.ingestchan = ingestchan
}

func main() {
	if logfile, err := os.OpenFile(LOG_FILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664); err != nil {
		log.Fatalln("failed to open log file", err)
	} else {
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	ingestchan := make(chan *model.Workflow, 10)
	defer close(ingestchan)

	omdbapi := gomdb.Init(API_KEY)
	dh := &discHandler{}
	discdb := drive.NewJsonDiscDatabase(path.Join(OUT_DIR, "discs.json"))
	driveman := drive.NewUdevDriveManager(dh.handleDisc)
	wfman := model.NewWorkflowManager(path.Join(OUT_DIR, "workflows.json"))

	dh.init(discdb, driveman, wfman, omdbapi, ingestchan)

	driveman.Start()
	defer driveman.Stop()

	for _, workflow := range wfman.GetWorkflows() {
		go func(w *model.Workflow) {
			if w.Name != nil && w.Year != nil {
				ingestchan <- w
			}
		}(workflow)
	}

	go handleIngestRequests(wfman, targets, ingestchan)

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	indexHandler := handler.NewIndexHandler(driveman, wfman)
	driveHandler := handler.NewDriveHandler(discdb, driveman, omdbapi)
	workflowHandler := handler.NewWorkflowHandler(wfman, ingestchan)

	server.GET("/", indexHandler.GetIndex)
	server.GET("/drive", driveHandler.GetDrive)
	server.GET("/workflow/:id", workflowHandler.GetWorkflow)
	server.POST("/workflow/:id", workflowHandler.PostWorkflow)

	go func() {
		if err := server.Start(":8080"); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalln("server error", err)
		}
	}()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
	<-sigchan

	log.Println("shutting down")
	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalln("server shutdown error", err)
	}
}

func handleIngestRequests(workflowManager model.WorkflowManager, targets []string, inchan <-chan *model.Workflow) {
	for workflow := range inchan {
		log.Println("ingesting", workflow)

		mkv := workflow.File
		if mkv == nil {
			log.Println("no files to ingest")
			return
		}

		workflow.Status = model.StatusImporting
		workflowManager.Save(workflow)

		var err error
		for _, target := range targets {
			ingester, err := ingest.NewIngester(target)
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
			workflowManager.Clean(workflow)
			workflow.Status = model.StatusDone
			workflowManager.Save(workflow)
		}
	}
}

func (h *discHandler) handleDisc(disc *drive.Disc) {
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
		movie, err := h.omdbapi.MovieByTitle(&gomdb.QueryData{Title: main.Name})
		if err != nil {
			log.Println("error fetching movie:", main.Name, "err:", err)
			return
		}

		wf, _ := h.workflowManager.NewWorkflow(disc.Uuid, disc.Label)
		wf.Label = disc.Label
		wf.Name = &movie.Title
		wf.Year = &movie.Year
		wf.Status = model.StatusRipping
		h.workflowManager.Save(wf)

		dir := path.Join(OUT_DIR, wf.Id)
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			log.Println("error making dir:", dir, "err:", err)
			return
		}

		f, err := h.driveManager.RipFile(main, dir)
		if err != nil {
			log.Println("error ripping:", wf, "err:", err)
			return
		}

		wf.File = f
		wf.Status = model.StatusPending
		h.workflowManager.Save(wf)

		h.ingestchan <- wf
	}
}
