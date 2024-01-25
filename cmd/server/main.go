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
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const LOG_FILE = "./mkv.log"
const OUT_DIR = "."

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

	ingestchan := make(chan *model.Workflow, 10)
	defer close(ingestchan)

	discdb := drive.NewJsonDiscDatabase(path.Join(OUT_DIR, "discs.json"))
	driveManager := drive.NewUdevDeviceManager(discdb)
	driveManager.Start()
	defer driveManager.Stop()

	workflowManager := model.NewWorkflowManager(path.Join(OUT_DIR, "workflows.json"))
	for _, workflow := range workflowManager.GetWorkflows() {
		go func(w *model.Workflow) {
			if w.Name != nil && w.Year != nil {
				ingestchan <- w
			}
		}(workflow)
	}

	go handleIngestRequests(workflowManager, targets, ingestchan)

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	indexHandler := handler.NewIndexHandler(driveManager, workflowManager)
	driveHandler := handler.NewDriveHandler(driveManager)
	workflowHandler := handler.NewWorkflowHandler(workflowManager, ingestchan)

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
