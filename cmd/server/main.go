package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
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

const RIP_DIR = "/var/rip"
const LOCAL_DIR = "/mnt/nas/plex"
const REMOTE_DIR = "ssh://plexbot/plex"

func main() {
	if logfile, err := os.OpenFile("mkv.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664); err != nil {
		log.Fatalln("failed to open log file", err)
	} else {
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	devchan := make(chan *UdevDevice)
	detailchan := make(chan *model.Workflow, 10)
	ingestchan := make(chan *model.Workflow, 10)

	listener := NewUdevListener(devchan)
	listener.Start()
	defer listener.Stop()

	server := &http.Server{
		Addr: ":8080",
	}

	targets := []string{REMOTE_DIR, LOCAL_DIR}

	go handleDevices(RIP_DIR, devchan, detailchan)
	go handleIngestRequests(targets, ingestchan)

	for _, workflow := range loadExistingWorkflows(RIP_DIR) {
		go func(w *model.Workflow) {
			if w.Name == nil || w.Year == nil {
				detailchan <- w
			} else {
				ingestchan <- w
			}
		}(workflow)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "Hello world\n")
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

	close(devchan)
	close(detailchan)
	close(ingestchan)
}

func loadExistingWorkflows(dir string) []*model.Workflow {
	files, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = os.MkdirAll(dir, 0775)
			return []*model.Workflow{}
		}
		log.Fatal(err)
	}

	result := []*model.Workflow{}
	for _, file := range files {
		ext := path.Ext(file.Name())
		if ext == ".json" {
			log.Println("Found existing file:", file)
			workflow, err := model.LoadWorkflow(path.Join(dir, file.Name()))
			if err != nil {
				log.Println("Failed to load workflow:", file, err)
				continue
			}

			result = append(result, workflow)
		}
	}
	return result
}

func handleDevices(dir string, devchan <-chan *UdevDevice, outchan chan<- *model.Workflow) {
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

			go func(w *model.Workflow) {
				outchan <- w
			}(workflow)
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