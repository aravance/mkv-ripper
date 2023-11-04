package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

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

			var sums map[string]string
			for _, file := range files {
				oldfile := filepath.Join(dir, file.Name())
				u := uuid.New()
				newfile := filepath.Join(newdir, fmt.Sprintf("%s.mkv", u))
				os.Rename(oldfile, newfile)
				sums[u.String()], _ = shasum(newfile)
			}

			content := map[string]interface{}{
				"name":  details.name,
				"year":  details.year,
				"files": sums,
			}
			newfile := filepath.Join(newdir, uuid.New().String()+".in")
			bytes, _ := json.Marshal(content)
			if err := os.WriteFile(newfile, bytes, 0664); err != nil {
				log.Fatal(err)
			}
		}
	})
}
