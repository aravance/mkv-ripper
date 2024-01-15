package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
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

	go handleDevices(RIP_DIR, devchan)
	go handleIngestRequests(targets, ingestchan)

	model.SetDir(RIP_DIR)
	for _, workflow := range model.LoadExistingWorkflows() {
		go func(w *model.Workflow) {
			if w.Name != nil && w.Year != nil {
				ingestchan <- w
			}
		}(workflow)
	}

	server := echo.New()

	server.Use(middleware.Logger())
	server.Use(middleware.Recover())

	workflowHandler := handler.NewWorkflowHandler(ingestchan)

	server.GET("/", index)
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

func index(c echo.Context) error {
	workflows := model.LoadExistingWorkflows()
	if len(workflows) == 0 {
		return c.String(http.StatusOK, fmt.Sprintf("No workflows in progress"))
	} else {
		workflowStrs := make([]string, len(workflows))
		for i, workflow := range workflows {
			workflowStrs[i] = fmt.Sprintf("%s: %s", workflow.Id, workflow.Label)
		}
		return c.String(http.StatusOK, strings.Join(workflowStrs, "\n\n"))
	}
}

func handleDevices(dir string, devchan <-chan *UdevDevice) {
	for dev := range devchan {
		if dev.Available() {
			workflow := model.NewWorkflow(uuid.New().String(), dir, dev.Label())

			if files, err := ripFiles(dev, workflow.Id, dir); err != nil {
				log.Println("Error ripping device", err)
				continue
			} else {
				workflow.AddFiles(files...)

				if err := workflow.Save(); err != nil {
					log.Println("Failed to save workflow", workflow, err)
					continue
				}
			}
		} else {
			log.Println("Unavailable device", dev)
		}
	}
}

func handleIngestRequests(targets []string, inchan <-chan *model.Workflow) {
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

			err = ingester.Ingest(workflow)
			if err != nil {
				log.Println("error running ingester", ingester, err)
			}
		}

		if err == nil {
			log.Println("removing files")
			for _, file := range workflow.Files {
				os.Remove(path.Join(workflow.Dir, file.Filename))
			}
			os.Remove(workflow.JsonFile())
		}
	}
}
