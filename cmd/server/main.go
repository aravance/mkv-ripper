package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/handler"
	"github.com/aravance/mkv-ripper/ingest"
	"github.com/aravance/mkv-ripper/model"
	"github.com/eefret/gomdb"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	config := parseConfig()

	if logfile, err := os.OpenFile(path.Join(config.Log, "mkv.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664); err != nil {
		log.Fatalln("failed to open log file", err)
	} else {
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	dh := &ingest.IngestHandler{}
	discdb := drive.NewJsonDiscDatabase("discs.json")
	driveman := drive.NewUdevDriveManager(dh.HandleDisc)
	wfman := model.NewJsonWorkflowManager("workflows.json")
	omdbapi := gomdb.Init(config.Omdb.Apikey)

	targets := make([]*url.URL, len(config.Targets))
	for i, t := range config.Targets {
		targets[i] = &url.URL{
			Scheme: t.Scheme,
			Host:   t.Host,
			Path:   t.Path,
		}
	}
	dh.Init(discdb, driveman, wfman, omdbapi, targets, config.Rip)

	driveman.Start()
	defer driveman.Stop()

	for _, workflow := range wfman.GetWorkflows() {
		if workflow.Status == model.StatusImporting {
			go func(w *model.Workflow) {
				if w.Name != nil && w.Year != nil {
					dh.IngestWorkflow(w)
				}
			}(workflow)
		}
	}

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	indexHandler := handler.NewIndexHandler(driveman, wfman)
	driveHandler := handler.NewDriveHandler(discdb, driveman, wfman, omdbapi)
	workflowHandler := handler.NewWorkflowHandler(wfman, dh)
	omdbHandler := handler.NewOmdbHandler(omdbapi)

	server.GET("/", indexHandler.GetIndex)
	server.GET("/drive", driveHandler.GetDrive)
	server.GET("/drive/status", driveHandler.GetDriveStatus)
	server.GET("/workflow/:id", workflowHandler.GetWorkflow)
	server.POST("/workflow/:id", workflowHandler.PostWorkflow)
	server.GET("/omdb/:query", omdbHandler.Search)

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
