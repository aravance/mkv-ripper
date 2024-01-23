package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
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

func ripFile(device makemkv.Device, titleId int, outdir string) (*model.MkvFile, error) {
	ripdir, err := os.MkdirTemp(outdir, ".rip")
	if err != nil {
		log.Println("failed to make temp dir", err)
		return nil, err
	}
	defer os.RemoveAll(ripdir)

	opts := makemkv.MkvOptions{
		Progress:  makemkv.Stropt("-same"),
		Minlength: makemkv.Intopt(3600),
		Noscan:    true,
	}
	log.Println("starting makemkv")
	mkvjob := makemkv.Mkv(device, titleId, ripdir, opts)

	statchan := make(chan makemkv.Status)
	mkvjob.Statuschan = statchan

	log.Println("processing makemkv output")
	for status := range statchan {
		log.Println(status)
	}

	if err := mkvjob.Run(); err != nil {
		log.Println("error ripping device", err)
		return nil, err
	}

	if files, err := os.ReadDir(ripdir); err != nil {
		log.Println("error opening dir", ripdir, err)
		return nil, err
	} else {
		if len(files) == 0 {
			log.Println("no files found after ripping")
			return nil, nil
		} else if len(files) > 1 {
			log.Println("too many files found after ripping")
			return nil, nil
		} else {
			file := files[0]
			oldfile := path.Join(ripdir, file.Name())
			log.Println("starting sha256sum for " + oldfile)
			shasum, err := util.Sha256sum(oldfile)
			if err != nil {
				log.Println("error in sha256sum for " + oldfile)
				return nil, err
			} else {
				log.Println("sha256sum " + file.Name() + ": " + shasum)
			}

			newfile := path.Join(outdir, file.Name())
			os.Rename(oldfile, newfile)
			resolution, err := getResolution(newfile)
			if err != nil {
				log.Println("error getting resolution for "+newfile, err)
				return nil, err
			}

			return &model.MkvFile{
				Filename:   newfile,
				Shasum:     shasum,
				Resolution: resolution,
			}, nil
		}
	}
}
