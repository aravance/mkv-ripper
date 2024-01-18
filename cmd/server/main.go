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

	"github.com/aravance/mkv-ripper/handler"
	"github.com/aravance/mkv-ripper/ingest"
	"github.com/aravance/mkv-ripper/model"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const LOG_FILE = "./mkv.log"
const RIP_DIR = "/var/rip"
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

	devchan := make(chan *UdevDevice)
	defer close(devchan)

	ingestchan := make(chan *model.Workflow, 10)
	defer close(ingestchan)

	listener := NewUdevListener(devchan)
	listener.Start()
	defer listener.Stop()

	workflowManager := model.NewWorkflowManager(path.Join(OUT_DIR, "workflows.json"))
	for _, workflow := range workflowManager.GetWorkflows() {
		go func(w *model.Workflow) {
			if w.Name != nil && w.Year != nil {
				ingestchan <- w
			}
		}(workflow)
	}

	go handleDevices(workflowManager, devchan)
	go handleIngestRequests(workflowManager, targets, ingestchan)

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	indexHandler := handler.NewIndexHandler(workflowManager)
	workflowHandler := handler.NewWorkflowHandler(workflowManager, ingestchan)

	server.GET("/", indexHandler.GetIndex)
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

func handleDevices(workflowManager model.WorkflowManager, devchan <-chan *UdevDevice) {
	for dev := range devchan {
		if dev.Available() {
			id := uuid.New().String()
			workflow := workflowManager.NewWorkflow(id, dev.Label())

			dir := path.Join(OUT_DIR, id)
			if err := os.MkdirAll(dir, 0775); err != nil {
				log.Println("Error making file directory", err)
				continue
			}

			if files, err := ripFiles(dev, RIP_DIR, dir); err != nil {
				log.Println("Error ripping device", err)
				continue
			} else {
				workflow.Files = append(workflow.Files, files...)

				if err := workflowManager.Save(workflow); err != nil {
					log.Println("Failed to save workflow", workflow, err)
					continue
				}
			}
		} else {
			log.Println("Unavailable device", dev)
		}
	}
}

func handleIngestRequests(workflowManager model.WorkflowManager, targets []string, inchan <-chan *model.Workflow) {
	for workflow := range inchan {
		log.Println("Ingesting", workflow)

		if len(workflow.Files) == 0 {
			log.Println("no files to ingest")
		}
		if len(workflow.Files) > 1 {
			log.Println("too many files to ingest")
		}

		var err error
		for _, target := range targets {
			ingester, err := ingest.NewIngester(target)
			if err != nil {
				log.Println("Error finding ingester", err, "for target", target)
				continue
			}

			mkv := workflow.Files[0]
			err = ingester.Ingest(mkv, *workflow.Name, *workflow.Year)
			if err != nil {
				log.Println("error running ingester", ingester, err)
			}
		}

		if err == nil {
			log.Println("cleaning workflow")
			workflowManager.Clean(workflow)
		}
	}
}
