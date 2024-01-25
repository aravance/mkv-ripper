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

	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/handler"
	"github.com/aravance/mkv-ripper/ingest"
	"github.com/aravance/mkv-ripper/model"
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

func main() {
	if logfile, err := os.OpenFile(LOG_FILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664); err != nil {
		log.Fatalln("failed to open log file", err)
	} else {
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	dh := &ingest.IngestHandler{}
	discdb := drive.NewJsonDiscDatabase(path.Join(OUT_DIR, "discs.json"))
	driveman := drive.NewUdevDriveManager(dh.HandleDisc)
	wfman := model.NewWorkflowManager(path.Join(OUT_DIR, "workflows.json"))
	omdbapi := gomdb.Init(API_KEY)

	dh.Init(discdb, driveman, wfman, omdbapi, targets, OUT_DIR)

	driveman.Start()
	defer driveman.Stop()

	for _, workflow := range wfman.GetWorkflows() {
		go func(w *model.Workflow) {
			if w.Name != nil && w.Year != nil {
				dh.IngestWorkflow(w)
			}
		}(workflow)
	}

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	indexHandler := handler.NewIndexHandler(driveman, wfman)
	driveHandler := handler.NewDriveHandler(discdb, driveman, omdbapi)
	workflowHandler := handler.NewWorkflowHandler(wfman, dh)

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
