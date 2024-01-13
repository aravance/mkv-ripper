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

type DetailRequest struct {
	jsonfile string
}

type IngestRequest struct {
	jsonfile string
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

	dir := defaultDir

	go func() {
		for dev := range devchan {
			if dev.Available() {
				label := dev.Label()
				log.Println("Found device:", label)

				workflow := NewWorkflow(
					uuid.New().String(),
					path.Join(dir, ".input"),
					label,
				)

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
					detailchan <- w
				}(workflow)
			} else {
				log.Println("Unavailable device", dev)
			}
		}
	}()

	go func() {
		for workflow := range detailchan {
			if workflow.Name == nil || workflow.Year == nil {
				details := requestDetails(workflow)
				workflow.AddMovieDetails(details)
				if err := workflow.Save(); err != nil {
					log.Println("Failed to save workflow", workflow, err)
					continue
				}
			}

			go func(w *Workflow) {
				ingestchan <- w
			}(workflow)
		}
	}()

	go func() {
		for workflow := range ingestchan {
			for _, file := range workflow.Files {
				filename := path.Join(workflow.Dir, file.Filename)
				fmt.Println("mkv file:", path.Join(workflow.Dir, filename))
				continue
				cmd := exec.Command("scp", filename, "plexbot:~")
				cmd.Start()
				cmd.Wait()
			}
			fmt.Println("Ingest workflow", workflow, workflow.JsonFile())
			continue

			// TODO scp json file
			cmd := exec.Command("scp", workflow.JsonFile(), "plexbot:~")
			cmd.Start()
			cmd.Wait()

			// TODO run ssh ingest
			cmd = exec.Command("ssh", "plexbot", "./ingest.sh", path.Base(workflow.JsonFile()))
			cmd.Start()
			cmd.Wait()

			// TODO run local ingest
			cmd = exec.Command("/mnt/nas/plex/ingest.sh", "plexbot", "./ingest.sh", workflow.JsonFile())
			cmd.Start()
			cmd.Wait()
		}
	}()

	go func() {
		inpath := path.Join(dir, ".input")
		files, err := os.ReadDir(inpath)
		if err != nil {
			log.Fatal(err)
		}

		for _, file := range files {
			ext := path.Ext(file.Name())
			fmt.Println(file, ext)
			if ext == ".json" {
				log.Println("Found existing file:", file)
				workflow, err := LoadWorkflow(path.Join(inpath, file.Name()))
				if err != nil {
					log.Println("Failed to load workflow:", file, err)
					continue
				}

				go func(w *Workflow) {
					detailchan <- w
				}(workflow)
			}
		}
	}()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan

	log.Println("Shutting down")
	close(devchan)
}
