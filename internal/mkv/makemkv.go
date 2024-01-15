package mkv

import (
	"bufio"
	"errors"
	"io/fs"
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

type MkvOptions struct {
	Messages  *string
	Progress  *string
	Debug     *string
	Directio  *bool
	Cache     *string
	Minlength *int
	Noscan    bool
}

func (m MkvOptions) toStrings() []string {
	result := []string{"-r"}
	if m.Messages != nil {
		result = append(result, "--messages="+*m.Messages)
	}
	if m.Progress != nil {
		result = append(result, "--progress="+*m.Progress)
	}
	if m.Debug != nil {
		result = append(result, "--debug="+*m.Debug)
	}
	if m.Directio != nil {
		result = append(result, "--directio="+strconv.FormatBool(*m.Directio))
	}
	if m.Minlength != nil {
		result = append(result, "--minlength"+strconv.Itoa(*m.Minlength))
	}
	if m.Noscan {
		result = append(result, "--noscan")
	}
	return result
}

func Stropt(s string) *string {
	return &s
}

func Intopt(i int) *int {
	return &i
}

func Mkv(device Device, titleId string, destination string, opts MkvOptions) (chan RipStatus, error) {
	dev := device.Type() + ":" + device.Device()
	options := append(opts.toStrings(), []string{"mkv", dev, titleId, destination}...)
	cmd := exec.Command("makemkvcon", options...)

	var scanner bufio.Scanner
	if out, err := cmd.StdoutPipe(); err != nil {
		return nil, err
	} else {
		scanner = *bufio.NewScanner(out)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	statuschan := make(chan RipStatus)

	go func() {
		defer close(statuschan)
		var title string
		var channel string
		var total int
		var current int
		var max int
		for scanner.Scan() {
			line := scanner.Text()
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
		if err := cmd.Wait(); err != nil {
			// TODO what do I do with this err?
		}
	}()
	return statuschan, nil
}
