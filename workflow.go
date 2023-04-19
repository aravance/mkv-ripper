package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
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
		go func() {
			for status := range statchan {
				w.status = &status
			}
		}()
		details := w.deviceHandler.HandleDevice(w.device)

		log.Printf("Rip: %+v\n", details)
		log.Println("Created", w.path)
		for stat := range statchan {
			fmt.Println(stat)
		}
		log.Println("Done")
		if files, err := ioutil.ReadDir(dir); err != nil {
			log.Println("Error opening dir", dir)
		} else {
			if len(files) > 1 {
				// TODO identify files
				log.Println("Too many files ripped")
				return
			}
			for _, file := range files {
				fullname := fmt.Sprintf("%s (%s)", details.name, details.year)
				oldfile := filepath.Join(dir, file.Name())
				newdir := filepath.Join(w.path, fullname)
				newfile := filepath.Join(newdir, fullname+".mkv")
				os.Mkdir(newdir, 0775)
				os.Rename(oldfile, newfile)

				shasum, err := shasum(newfile)
				if err != nil {
					log.Println("Error running shasum", err)
					return
				}
				shafile := filepath.Join(defaultPath, "Movies.sha256")
				path := fmt.Sprintf("Movies/%s/%s.mkv", fullname, fullname)
				addShasum(shafile, shasum, path)

				log.Printf("%s  Movies/%s/%s.mkv\n", shasum, fullname, fullname)
			}
		}
	})
}
