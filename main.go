package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
)

const defaultPath = "/var/rip"

type MovieDetails struct {
	name    string
	year    string
	variant string
}

type DetailRequest struct {
	infile string
}

type IngestRequest struct {
	infile  string
	details *MovieDetails
}

func readJson(file string) (map[string]interface{}, error) {
	bytes, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	content := map[string]interface{}{}
	json.Unmarshal(bytes, &content)
	return content, nil
}

func writeJson(file string, content map[string]interface{}) error {
	if bytes, err := json.Marshal(content); err != nil {
		return err
	} else if err := os.WriteFile(file, bytes, 0664); err != nil {
		return err
	} else {
		return nil
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
	detailchan := make(chan *DetailRequest)
	ingestchan := make(chan *IngestRequest)

	listener := NewUdevListener(devchan)
	listener.Start()
	defer listener.Stop()

	path := defaultPath

	go func() {
		for dev := range devchan {
			if dev.Available() {
				request := NewRipRequest(dev, path, detailchan)
				go request.Start()
			} else {
				log.Println("Unavailable device", dev)
			}
		}
	}()

	go func() {
		for request := range detailchan {
			content, err := readJson(request.infile)
			if err != nil {
				log.Fatal(err)
			}
			details, changed := requestDetails(content)
			if changed {
				if err := writeJson(request.infile, content); err != nil {
					log.Fatal(err)
				}
			}
			ingestRequest := &IngestRequest{request.infile, details}
			go func(request *IngestRequest) {
				ingestchan <- request
			}(ingestRequest)
		}
	}()

	go func() {
		for request := range ingestchan {
			fmt.Println("Ingest request:", request)
		}
	}()

	go func() {
		fmt.Println("Listing files")
		inpath := filepath.Join(path, ".input")
		files, err := os.ReadDir(inpath)
		if err != nil {
			log.Fatal(err)
		}
		for _, file := range files {
			fmt.Println("Working on file:", file)
			ext := filepath.Ext(file.Name())
			if ext == ".json" {
				detailchan <- &DetailRequest{filepath.Join(inpath, file.Name())}
			}
		}
	}()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan

	log.Println("Shutting down")
	close(devchan)
}
