package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/handler"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
	"github.com/aravance/mkv-ripper/workflow"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	cfg := ParseConfigFile("mkv-ripper.toml")
	outdir := cfg.Rip
	targets := make([]*url.URL, len(cfg.Targets))
	for i, t := range cfg.Targets {
		targets[i] = &url.URL{
			Scheme: t.Scheme,
			Host:   t.Host,
			Path:   t.Path,
		}
	}

	if logfile, err := os.OpenFile(path.Join(cfg.Log, "mkv.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664); err != nil {
		log.Fatalln("failed to open log file", err)
	} else {
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	var wfman workflow.WorkflowManager
	omdbapi := gomdb.Init(cfg.Omdb.Apikey)
	discdb := drive.NewJsonDiscDatabase("discs.json")
	handle := func(driveman drive.DriveManager) {
		handleDisc(discdb, wfman, driveman, omdbapi)
	}
	driveman := drive.NewUdevDriveManager(handle)
	wfman = workflow.NewJsonWorkflowManager(driveman, discdb, targets, outdir, "workflows.json")

	driveman.Start()
	defer driveman.Stop()

	for _, wf := range wfman.GetAllWorkflows() {
		if wf.Status == model.StatusRipping {
			wf.Status = model.StatusError
			wfman.Save(wf)
		}
		if wf.Status == model.StatusImporting {
			wf.Status = model.StatusPending
			wfman.Save(wf)
		}
		if wf.Status == model.StatusPending {
			if wf.Name != nil && wf.Year != nil {
				go func(w *model.Workflow) {
					wfman.Ingest(w)
				}(wf)
			}
		}
	}

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	indexHandler := handler.NewIndexHandler(driveman, wfman)
	driveHandler := handler.NewDriveHandler(discdb, driveman, wfman, omdbapi)
	workflowHandler := handler.NewWorkflowHandler(wfman, driveman, discdb, omdbapi)
	omdbHandler := handler.NewOmdbHandler(omdbapi)

	server.GET("/", indexHandler.GetIndex)
	server.GET("/drive", driveHandler.GetDrive)
	server.GET("/drive/status", driveHandler.GetDriveStatus)
	server.GET("/disc/:discId/title/:titleId", workflowHandler.GetWorkflow)
	server.POST("/disc/:discId/title/:titleId", workflowHandler.PostWorkflow)
	server.GET("/disc/:discId/title/:titleId/edit", workflowHandler.EditWorkflow)
	server.GET("/disc/:discId/title/:titleId/status", workflowHandler.Status)
	// TODO make this a post
	server.GET("/disc/:discId/title/:titleId/rip", workflowHandler.RipTitle)
	server.GET("/omdb/search", omdbHandler.Search)

	go func() {
		if err := server.Start(fmt.Sprintf(":%d", cfg.Port)); !errors.Is(err, http.ErrServerClosed) {
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

func handleDisc(
	discdb drive.DiscDatabase,
	wfman workflow.WorkflowManager,
	driveman drive.DriveManager,
	omdbapi *gomdb.OmdbApi,
) {
	disc := driveman.GetDisc()
	if disc == nil || (disc.Uuid == "" && disc.Label == "") {
		return
	}

	var info *makemkv.DiscInfo
	var found bool
	var err error
	info, found = discdb.GetDiscInfo(disc.Uuid)
	if !found {
		info, err = driveman.GetDiscInfo()
		if err != nil {
			log.Println("error getting disc info:", disc, "err:", err)
			return
		}
		err = discdb.SaveDiscInfo(disc.Uuid, info)
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
		wf, _ := wfman.NewWorkflow(disc.Uuid, main.Id, disc.Label, name)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if movie, err := util.GetMovie(name, omdbapi); err != nil {
				log.Println("failed to fetch movie details:", name)
			} else {
				wf.Name = &movie.Title
				wf.Year = &movie.Year
				wf.ImdbId = &movie.ImdbID
				wfman.Save(wf)
			}
		}()

		err = wfman.Start(wf)
		wg.Wait()
		if err != nil {
			log.Println("failed to rip title", err)
			return
		}
	}
}
