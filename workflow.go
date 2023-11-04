package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
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
		logchan := make(chan *RipStatus, 100000)
		go func() {
			for status := range statchan {
				w.status = &status
				select {
				case logchan <- &status:
				default:
				}
			}
			close(logchan)
		}()
		details := w.deviceHandler.HandleDevice(w.device)

		log.Printf("Rip: %+v\n", details)
		log.Println("Created", w.path)

		var progress int = 0
		var title string = ""
		var wait float64 = 200
		for status := range logchan {
			curr := 100 * status.current / status.max
			if title != status.title {
				fmt.Println()
				fmt.Println(status.title)
				title = status.title
				progress = 0
				wait = 200
			} else if curr > progress {
				fmt.Print(".", curr)
				progress = curr
				wait = 200
			} else if curr < 100 {
				fmt.Print(".")
				wait = math.Min(2 * wait, 2000)
				time.Sleep(time.Duration(int64(wait) * time.Millisecond.Milliseconds()))
			}
		}
		log.Println("Done")
		if files, err := ioutil.ReadDir(dir); err != nil {
			log.Println("Error opening dir", dir)
		} else {
			fullname := fmt.Sprintf("%s (%s)", details.name, details.year)
			if len(files) > 1 {
				log.Println("Too many files ripped")
				newdir := filepath.Join(w.path, "." + fullname)
				os.Mkdir(newdir, 0755)
				var i int = 1
				for _, file := range files {
					oldfile := filepath.Join(dir, file.Name())
					newfile := filepath.Join(newdir, fmt.Sprintf("%s [%d].mkv", fullname, i))
					os.Rename(oldfile, newfile)
					i++
				}
			} else {
				for _, file := range files { 
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
					path := fmt.Sprintf("%s/%s.mkv", fullname, fullname)
					addShasum(shafile, shasum, path)

					log.Printf("%s  %s/%s.mkv\n", shasum, fullname, fullname)
				}
			}
		}
	})
}
