package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
)

type Device interface {
	Label() string
	Dev() string
	Type() string
	Available() bool
}

type Rip struct {
	path string
	name string
	year string
	dev  Device
}

type DeviceHandler interface {
	HandleDevice(dev Device)
}

type RipStatus struct {
	title   string
	channel string
	current int
	max     int
}

func ripDisk(r Rip) error {
	dev := fmt.Sprintf("%s:%s", r.dev.Type(), r.dev.Dev())
	dir, err := os.MkdirTemp(r.path, r.name)
	if err != nil {
		fmt.Println("Failed to make temp directory")
		return err
	}
	defer os.RemoveAll(dir)
	cmd := exec.Command("makemkvcon", "-r", "--noscan", "--progress=-same", "mkv", dev, "0", dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("Failed to call makemkvcon")
		return err
	}
	scanner := bufio.NewScanner(out)
	go cmd.Start()

	f, err := os.CreateTemp(".", r.name)
	if err != nil {
		fmt.Println("Failed to create log file")
		return err
	}
	defer f.Close()
	defer os.Remove(f.Name())

	var title string
	var channel string
	var current int
	var max int
	for scanner.Scan() {
		line := scanner.Text()
		f.WriteString(line + "\n")
		prefix, content, _ := strings.Cut(line, ":")
		parts := strings.Split(content, ",")
		switch prefix {
		case "PRGT":
			title = parts[2]
		case "PRGC":
			channel = parts[2]
		case "PRGV":
			current, _ = strconv.Atoi(parts[0])
			max, _ = strconv.Atoi(parts[2])
			status := RipStatus{
				title:   title,
				channel: channel,
				current: current,
				max:     max,
			}
			fmt.Println(status)
		}
	}
	fmt.Println("Done.")
	return nil
}

func waitForShutdown() {
	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan

	fmt.Println("Shutting down")
}

const defaultPath = "/var/rip"

func main() {
	devchan := make(chan Device)
	ripchan := make(chan Rip)

	u := NewUdevListener(devchan)
	go u.Start()
	defer u.Stop()

	i := NewCliDeviceHandler(defaultPath, devchan, ripchan)
	go func() {
		for dev := range devchan {
			i.HandleDevice(dev)
		}
	}()

	go func() {
		for rip := range ripchan {
			fmt.Printf("Rip: %+v\n", rip)
			ripDisk(rip)
		}
	}()

	waitForShutdown()
}
