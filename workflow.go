package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
)

type DeviceHandler interface {
	HandleDevice(Device) *MovieDetails
}

type Workflow struct {
	deviceHandler DeviceHandler
	device        Device
	path          string
	once          sync.Once
	details       *MovieDetails
	status        *RipStatus
}

func NewWorkflow(deviceHandler DeviceHandler, device Device, path string) Workflow {
	return Workflow{
		deviceHandler: deviceHandler,
		device:        device,
		path:          path,
		once:          sync.Once{},
		details:       nil,
	}
}

func (w *Workflow) Start() {
	go w.once.Do(func() {
		dir, err := os.MkdirTemp(w.path, ".rip")
		if err != nil {
			log.Println("Failed to make temp dir", err)
			return
		}
		defer os.RemoveAll(dir)
		statchan, err := ripDevice(w.device, dir)
		if err != nil {
			log.Println("Error ripping device", err)
			return
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for status := range statchan {
				w.status = &status
			}
		}()
		details := w.deviceHandler.HandleDevice(w.device)

		log.Printf("Rip: %+v\n", details)
		log.Println("Created", w.path)

		wg.Wait()
		log.Println("Done")
		if files, err := ioutil.ReadDir(dir); err != nil {
			log.Println("Error opening dir", dir)
		} else {
			newdir := filepath.Join(w.path, ".input")
			if stat, err := os.Stat(newdir); errors.Is(err, os.ErrNotExist) {
				if err := os.Mkdir(newdir, 0755); err != nil {
					log.Fatal(err)
					return
				}
			} else if !stat.IsDir() {
				log.Fatal(".input is not a directory")
				return
			}

			fileDetails := map[string]interface{}{}
			for _, file := range files {
				oldfile := filepath.Join(dir, file.Name())
				log.Println("Starting shasum for " + oldfile)
				shasum, err := shasum(oldfile)
				if err != nil {
					log.Println("Error in shasum for " + oldfile)
					continue
				}
				log.Println("Shasum " + file.Name() + ":" + shasum)
				u := uuid.New()
				newfile := filepath.Join(newdir, u.String()+".mkv")
				os.Rename(oldfile, newfile)
				fileDetails[u.String()] = map[string]interface{}{
					"shasum": shasum,
					"name":   file.Name(),
				}
			}

			content := map[string]interface{}{
				"name":  details.name,
				"year":  details.year,
				"files": fileDetails,
			}
			newfile := filepath.Join(newdir, uuid.New().String()+".json")
			if bytes, err := json.Marshal(content); err != nil {
				log.Fatal(err)
			} else if err := os.WriteFile(newfile, bytes, 0664); err != nil {
				log.Fatal(err)
			}
		}
	})
}
