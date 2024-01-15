package mkv

import (
	"bufio"
	"errors"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Device interface {
	Device() string
	Type() string
	Available() bool
}

type IsoDevice struct {
	path string
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
	path string
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

type DevDevice struct {
	device string
}

func (d *DevDevice) Device() string {
	return "/dev/" + d.device
}

func (d *DevDevice) Type() string {
	return "disc"
}

func (d *DevDevice) Available() bool {
	_, err := os.Stat(d.Device())
	return err == nil || !errors.Is(err, fs.ErrNotExist) 
}

type DiscDevice struct {
	id int
}

func (d *DiscDevice) Device() string {
	return strconv.Itoa(d.id)
}

func (d *DiscDevice) Type() string {
	return "dev"
}

func (d *DiscDevice) Available() bool {
	panic("not yet implemented")
}

type RipStatus struct {
	channel string
	title   string
	current int
	total   int
	max     int
}

func Mkv(device Device, titleId string, path string) (chan RipStatus, error) {
	dev := device.Type() + ":" + device.Device()
	cmd := exec.Command("makemkvcon", "-r", "--noscan", "--progress=-same", "--minlength=3600", "mkv", dev, titleId, path)
	var scanner bufio.Scanner
	if out, err := cmd.StdoutPipe(); err != nil {
		log.Println("Failed to call makemkvcon")
		return nil, err
	} else {
		scanner = *bufio.NewScanner(out)
	}
	log.Println("Ripping")
	if err := cmd.Start(); err != nil {
		log.Fatalln("Failed to call makemkvcon")
	}

	statuschan := make(chan RipStatus)

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
