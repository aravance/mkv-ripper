package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/vansante/go-ffprobe"
)

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

func ripFiles(device Device, workflowId string, dir string) ([]MkvFile, error) {
	ripdir, err := os.MkdirTemp(dir, ".rip")
	if err != nil {
		log.Println("Failed to make temp dir", err)
		return nil, err
	}
	defer os.RemoveAll(ripdir)

	log.Println("Done")
	statchan, err := ripDevice(device, ripdir)
	if err != nil {
		log.Println("Error ripping device", err)
		return nil, err
	}

	for status := range statchan {
		log.Println(status)
	}

	if files, err := os.ReadDir(ripdir); err != nil {
		log.Println("Error opening dir", ripdir)
		return nil, err
	} else {
		fileDetails := make([]MkvFile, len(files))
		for i, file := range files {
			oldfile := path.Join(ripdir, file.Name())
			log.Println("Starting sha256sum for " + oldfile)
			shasum, err := sha256sum(oldfile)
			if err != nil {
				log.Fatal("Error in sha256sum for " + oldfile)
			} else {
				log.Println("sha256sum " + file.Name() + ": " + shasum)
			}

			mkvfile := fmt.Sprintf("%s_%02d.mkv", workflowId, i)
			newfile := path.Join(dir, mkvfile)
			os.Rename(oldfile, newfile)
			resolution, err := getResolution(newfile)
			if err != nil {
				log.Fatal("Error getting resolution for "+newfile, err)
			}

			fileDetails[i] = MkvFile{
				Filename:   mkvfile,
				Original:   file.Name(),
				Shasum:     shasum,
				Resolution: resolution,
			}
		}

		return fileDetails, nil
	}
}
