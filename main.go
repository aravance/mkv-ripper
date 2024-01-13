package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"

	"github.com/google/uuid"
)

const defaultDir = "/var/rip"
const remoteHost = "plexbot"
const remoteDir = "~"

func loadExistingWorkflows(dir string, outchan chan<- *Workflow) {
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		ext := path.Ext(file.Name())
		if ext == ".json" {
			log.Println("Found existing file:", file)
			workflow, err := LoadWorkflow(path.Join(dir, file.Name()))
			if err != nil {
				log.Println("Failed to load workflow:", file, err)
				continue
			}

			go func(w *Workflow) {
				outchan <- w
			}(workflow)
		}
	}
}

func handleDevices(dir string, devchan <-chan Device, outchan chan<- *Workflow) {
	for dev := range devchan {
		if dev.Available() {
			label := dev.Label()
			log.Println("Found device:", label)

			workflow := NewWorkflow(uuid.New().String(), dir, label)

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

			go func(w *Workflow) {
				outchan <- w
			}(workflow)
		} else {
			log.Println("Unavailable device", dev)
		}
	}
}

func handleDetailRequests(inchan <-chan *Workflow, outchan chan<- *Workflow) {
	for workflow := range inchan {
		if workflow.Name == nil || workflow.Year == nil {
			details := requestDetails(workflow)
			workflow.AddMovieDetails(details)
			if err := workflow.Save(); err != nil {
				log.Println("Failed to save workflow", workflow, err)
				continue
			}
		}

		go func(w *Workflow) {
			outchan <- w
		}(workflow)
	}
}

func handleIngestRequests(inchan <-chan *Workflow) {
	for workflow := range inchan {
		remote := fmt.Sprintf("%s:%s/.input", remoteHost, remoteDir)
		for _, file := range workflow.Files {
			filename := path.Join(workflow.Dir, file.Filename)
			cmd := exec.Command("scp", filename, remote)
			cmd.Start()
			cmd.Wait()
		}

		// TODO scp json file
		cmd := exec.Command("scp", workflow.JsonFile(), remote)
		cmd.Start()
		cmd.Wait()

		fmt.Println("Ingest workflow", workflow, workflow.JsonFile())
		continue

		// TODO run ssh ingest
		cmd = exec.Command("ssh", remoteHost, path.Join(remoteDir, "ingest.sh"), path.Base(workflow.JsonFile()))
		cmd.Start()
		cmd.Wait()

		// TODO run local ingest
		// check sha256sum
		// create directory
		// fix permissions
		// add sha256sum to Movies.sha256
		// move Files
		// delete old files
	}
}

func main() {
	logfile, err := os.OpenFile("mkv.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		log.Fatalln("Failed to open log file", err)
	}
	defer logfile.Close()
	log.SetOutput(logfile)

	devchan := make(chan Device)
	detailchan := make(chan *Workflow)
	ingestchan := make(chan *Workflow)

	listener := NewUdevListener(devchan)
	listener.Start()
	defer listener.Stop()

	go loadExistingWorkflows(defaultDir, detailchan)
	go handleDevices(defaultDir, devchan, detailchan)
	go handleDetailRequests(detailchan, ingestchan)
	go handleIngestRequests(ingestchan)

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan

	log.Println("Shutting down")
	close(devchan)
	close(detailchan)
	close(ingestchan)
}
