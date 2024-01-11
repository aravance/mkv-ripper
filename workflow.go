package main

import (
	"errors"
	"fmt"
	"io/fs"
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

type RipRequest struct {
	device  Device
	path    string
	once    sync.Once
	status  *RipStatus
	outchan chan *DetailRequest
}

func NewRipRequest(device Device, path string, outchan chan *DetailRequest) RipRequest {
	return RipRequest{
		device:  device,
		path:    path,
		outchan: outchan,
		once:    sync.Once{},
		status:  nil,
	}
}

func ripFiles(w *RipRequest, dir string) ([]fs.FileInfo, error) {
	statchan, err := ripDevice(w.device, dir)
	if err != nil {
		log.Println("Error ripping device", err)
		return nil, err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for status := range statchan {
			w.status = &status
		}
	}()

	wg.Wait()
	if files, err := ioutil.ReadDir(dir); err != nil {
		log.Println("Error opening dir", dir)
		return nil, err
	} else {
		return files, nil
	}
}

func (w *RipRequest) Start() {
	go w.once.Do(func() {
		dir, err := os.MkdirTemp(w.path, ".rip")
		if err != nil {
			log.Println("Failed to make temp dir", err)
			return
		}
		defer os.RemoveAll(dir)
		label := w.device.Label()

		log.Println("Done")
		if files, err := ripFiles(w, dir); err != nil {
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
				mkvfile := fmt.Sprintf("%s_%02d.mkv", u, i)
				newfile := filepath.Join(newdir, mkvfile)
				os.Rename(oldfile, newfile)
				fileDetails[i] = map[string]interface{}{
					"shasum": shasum,
					"name":   file.Name(),
					"file":   mkvfile,
				}
			}

			content := map[string]interface{}{
				"label": label,
				"files": fileDetails,
			}
			newfile := filepath.Join(newdir, u.String()+".json")
			if err := writeJson(newfile, content); err != nil {
				log.Fatal(err)
			} else {
				w.outchan <- &DetailRequest{newfile}
			}
		}
	})
}
