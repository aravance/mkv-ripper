package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
		status:        nil,
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
				if err := os.Mkdir(newdir, 0775); err != nil {
					log.Fatal(err)
					return
				}
			} else if !stat.IsDir() {
				log.Fatal(".input is not a directory")
				return
			}

			fileDetails := make([]interface{}, len(files))
			u := uuid.New()
			for i, file := range files {
				oldfile := filepath.Join(dir, file.Name())
				log.Println("Starting sha256sum for " + oldfile)
				shasum, err := sha256sum(oldfile)
				if err != nil {
					log.Println("Error in sha256sum for " + oldfile)
				} else {
					log.Println("sha256sum " + file.Name() + ": " + shasum)
				}
				mkvfile := fmt.Sprintf("%s[%d].mkv", u, i)
				newfile := filepath.Join(newdir, mkvfile)
				os.Rename(oldfile, newfile)
				fileDetails[i] = map[string]interface{}{
					"shasum": shasum,
					"name":   file.Name(),
					"index":  i,
				}
			}

			content := map[string]interface{}{
				"name":    details.name,
				"year":    details.year,
				"variant": details.variant,
				"files":   fileDetails,
			}
			newfile := filepath.Join(newdir, u.String()+".json")
			if bytes, err := json.Marshal(content); err != nil {
				log.Fatal(err)
			} else if err := os.WriteFile(newfile, bytes, 0664); err != nil {
				log.Fatal(err)
			}
		}
	})
}
