package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/vansante/go-ffprobe"
)

type DeviceHandler interface {
	HandleDevice(Device) *MovieDetails
}

func getResolution(file string) (string, error) {
	data, err := ffprobe.GetProbeData(file, 5*time.Second)
	if err != nil {
		return "", err
	}
	height := data.GetFirstVideoStream().Height
	switch height {
	case 2160:
		return "4k", nil
	default:
		return fmt.Sprintf("%dp", height), nil
	}
}

func ripFiles(device Device, path string) (*string, error) {
	dir, err := os.MkdirTemp(path, ".rip")
	if err != nil {
		log.Println("Failed to make temp dir", err)
		return nil, err
	}
	defer os.RemoveAll(dir)
	label := device.Label()

	log.Println("Done")
	statchan, err := ripDevice(device, dir)
	if err != nil {
		log.Println("Error ripping device", err)
		return nil, err
	}

	for status := range statchan {
		log.Println(status)
	}

	if files, err := os.ReadDir(dir); err != nil {
		log.Println("Error opening dir", dir)
		return nil, err
	} else {
		newdir := filepath.Join(path, ".input")
		if stat, err := os.Stat(newdir); errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(newdir, 0775); err != nil {
				log.Println("Error making directory", err)
				return nil, err
			}
		} else if !stat.IsDir() {
			log.Println(".input is not a directory")
			return nil, err
		}

		fileDetails := make([]interface{}, len(files))
		u := uuid.New()
		for i, file := range files {
			oldfile := filepath.Join(dir, file.Name())
			log.Println("Starting sha256sum for " + oldfile)
			shasum, err := sha256sum(oldfile)
			if err != nil {
				log.Fatal("Error in sha256sum for " + oldfile)
			} else {
				log.Println("sha256sum " + file.Name() + ": " + shasum)
			}
			mkvfile := fmt.Sprintf("%s_%02d.mkv", u, i)
			newfile := filepath.Join(newdir, mkvfile)
			os.Rename(oldfile, newfile)
			resolution, err := getResolution(newfile)
			if err != nil {
				log.Fatal("Error getting resolution for "+newfile, err)
			}
			fileDetails[i] = map[string]interface{}{
				"shasum":     shasum,
				"filename":   mkvfile,
				"original":   file.Name(),
				"resolution": resolution,
			}
		}

		content := map[string]interface{}{
			"label": label,
			"files": fileDetails,
		}
		newfile := filepath.Join(newdir, u.String()+".json")
		if err := writeJson(newfile, content); err != nil {
			log.Println("Failed to write json file", newfile, err)
			return nil, err
		}
		return &newfile, nil
	}
}
