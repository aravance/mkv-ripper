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

func main() {
	cfg := parseConfig()

	if logfile, err := os.OpenFile(path.Join(cfg.Log, "mkv.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664); err != nil {
		log.Fatalln("failed to open log file", err)
	} else {
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	wfman := model.NewJsonWorkflowManager("workflows.json")
	omdbapi := gomdb.Init(cfg.Omdb.Apikey)
	rip := func(d *drive.Disc, di *makemkv.DiscInfo, ti *makemkv.TitleInfo) (*model.Workflow, error) {
		return ripTitle(wfman, omdbapi, d, di, ti, cfg.Rip)
	}

	discdb := drive.NewJsonDiscDatabase("discs.json")
	ingesthandler := ingest.NewIngestHandler(wfman, targets(cfg))
	handle := func(disc *drive.Disc) {
		handleDisc(discdb, ingesthandler, disc, rip)
	}

	driveman := drive.NewUdevDriveManager(handle)

	driveman.Start()
	defer driveman.Stop()

	for _, workflow := range wfman.GetWorkflows() {
		if workflow.Status == model.StatusImporting {
			go func(w *model.Workflow) {
				if w.Name != nil && w.Year != nil {
					ingesthandler.IngestWorkflow(w)
				}
			}(workflow)
		}
	}

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	indexHandler := handler.NewIndexHandler(driveman, wfman)
	driveHandler := handler.NewDriveHandler(discdb, driveman, wfman, omdbapi)
	workflowHandler := handler.NewWorkflowHandler(wfman, ingesthandler)
	omdbHandler := handler.NewOmdbHandler(omdbapi)

	server.GET("/", indexHandler.GetIndex)
	server.GET("/drive", driveHandler.GetDrive)
	server.GET("/drive/status", driveHandler.GetDriveStatus)
	server.GET("/workflow/:id", workflowHandler.GetWorkflow)
	server.POST("/workflow/:id", workflowHandler.PostWorkflow)
	server.GET("/workflow/:id/edit", workflowHandler.EditWorkflow)
	server.GET("/omdb/search", omdbHandler.Search)

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

func targets(cfg Config) []*url.URL {
	targets := make([]*url.URL, len(cfg.Targets))
	for i, t := range cfg.Targets {
		targets[i] = &url.URL{
			Scheme: t.Scheme,
			Host:   t.Host,
			Path:   t.Path,
		}
	}
	return targets
}

func ripTitle(
	wfman model.WorkflowManager,
	omdbapi *gomdb.OmdbApi,
	disc *drive.Disc,
	di *makemkv.DiscInfo,
	ti *makemkv.TitleInfo,
	outdir string,
) (*model.Workflow, error) {
	if disc == nil {
		return nil, fmt.Errorf("disc cannot be nil")
	}
	if di == nil {
		return nil, fmt.Errorf("info cannot be nil")
	}
	if ti == nil {
		return nil, fmt.Errorf("title cannot be nil")
	}

	name := util.GuessName(di, ti)
	wf, _ := wfman.NewWorkflow(disc.Uuid, ti.Id, disc.Label, name)
	wf.Status = model.StatusRipping
	wfman.Save(wf)

	if movie, err := util.GetMovie(omdbapi, wf.OriginalName); err != nil {
		log.Println("failed to GetMovie", err)
	} else {
		wf.Name = &movie.Title
		wf.Year = &movie.Year
		wf.ImdbId = &movie.ImdbID
		wfman.Save(wf)
	}

	dir := path.Join(outdir, wf.Id())
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Println("error making dir:", dir, "err:", err)
		wf.Status = model.StatusError
		wfman.Save(wf)
		return nil, err
	}

	statchan := make(chan makemkv.Status)
	defer close(statchan)

	go func() {
		for stat := range statchan {
			disc.MkvStatus = &stat
		}
	}()

	f, err := disc.RipFile(ti, dir, statchan)
	if err != nil {
		log.Println("error ripping:", wf, "err:", err)
		wf.Status = model.StatusError
		wfman.Save(wf)
		return nil, err
	}

	wf.File = f
	wf.Status = model.StatusPending
	wfman.Save(wf)
	return wf, nil
}

func handleDisc(
	discdb drive.DiscDatabase,
	ingesthandler *ingest.IngestHandler,
	disc *drive.Disc,
	ripDisc func(*drive.Disc, *makemkv.DiscInfo, *makemkv.TitleInfo) (*model.Workflow, error),
) {
	if disc == nil {
		return
	}

	var info *makemkv.DiscInfo
	var found bool
	var err error
	info, found = discdb.GetDiscInfo(disc.Uuid)
	if !found {
		info, err = disc.GetDiscInfo()
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

		wf, err := ripDisc(disc, info, main)
		if err != nil {
			log.Println("failed to rip title", err)
			return
		}

		go ingesthandler.IngestWorkflow(wf)
	}
}
