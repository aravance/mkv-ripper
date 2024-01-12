package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Device interface {
	Label() string
	Device() string
	Type() string
	Available() bool
}

type IsoDevice struct {
	label string
	path  string
}

func (d *IsoDevice) Label() string {
	return d.label
}

func (d *IsoDevice) Device() string {
	return d.path
}

func (d *IsoDevice) Type() string {
	return "iso"
}

func (d *IsoDevice) Available() bool {
	info, err := os.Stat(d.path)
	return err == nil && !info.IsDir()
}

type FileDevice struct {
	label string
	path  string
}

func (d *FileDevice) Label() string {
	return d.label
}

func (d *FileDevice) Device() string {
	return d.path
}

func (d *FileDevice) Type() string {
	return "file"
}

func (d *FileDevice) Available() bool {
	info, err := os.Stat(d.path)
	return err == nil && info.IsDir()
}

type RipStatus struct {
	channel string
	title   string
	current int
	total   int
	max     int
}

func ripDevice(device Device, path string) (chan RipStatus, error) {
	dev := device.Type() + ":" + device.Device()
	cmd := exec.Command("makemkvcon", "-r", "--noscan", "--progress=-same", "--minlength=3600", "mkv", dev, "0", path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		log.Println("Failed to call makemkvcon")
		return nil, err
	}
	statuschan := make(chan RipStatus)

	log.Println("Ripping")
	scanner := bufio.NewScanner(out)
	if err := cmd.Start(); err != nil {
		log.Fatalln("Failed to call makemkvcon")
	}

	go func() {
		var title string
		var channel string
		var total int
		var current int
		var max int
		for scanner.Scan() {
			line := scanner.Text()
			// log.Println(line)
			prefix, content, _ := strings.Cut(line, ":")
			parts := strings.Split(content, ",")
			switch prefix {
			case "PRGT":
				title = parts[2]
			case "PRGC":
				channel = parts[2]
			case "PRGV":
				current, _ = strconv.Atoi(parts[0])
				total, _ = strconv.Atoi(parts[1])
				max, _ = strconv.Atoi(parts[2])
				statuschan <- RipStatus{
					title:   title,
					channel: channel,
					current: current,
					total:   total,
					max:     max,
				}
			}
		}
		log.Println("Finished ripping")
		if err := cmd.Wait(); err != nil {
			log.Println("Error finishing ripping", err)
		}
		close(statuschan)
	}()
	return statuschan, nil
}
