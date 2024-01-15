package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/aravance/mkv-ripper/internal/ingest"
	"github.com/aravance/mkv-ripper/internal/model"
	"github.com/google/uuid"
)

const LOG_FILE = "./mkv.log"
const RIP_DIR = "/var/rip"
const LOCAL_DIR = "/mnt/nas/plex"
const REMOTE_DIR = "ssh://plexbot/plex"

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

	targets := []string{REMOTE_DIR, LOCAL_DIR}

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

	server := &http.Server{
		Addr: ":8080",
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		workflows := model.LoadExistingWorkflows()
		if len(workflows) == 0 {
			io.WriteString(w, fmt.Sprintf("No workflows in progress"))
		} else {
			for _, workflow := range workflows {
				io.WriteString(w, fmt.Sprintf("%s: %s\n\n", workflow.Id, workflow.Label))
			}
		}
	})

	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
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

func handleDevices(dir string, devchan <-chan *UdevDevice) {
	for dev := range devchan {
		if dev.Available() {
			label := dev.Label()
			log.Println("Found device:", label)

			workflow := model.NewWorkflow(uuid.New().String(), dir, label)

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
