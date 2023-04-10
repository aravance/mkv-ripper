package main

import (
	"bufio"
	"fmt"
	"os"
)

type CliDeviceHandler struct {
	path string
	in   <-chan Device
	out  chan Rip
}

func NewCliDeviceHandler(path string, in <-chan Device, out chan Rip) *CliDeviceHandler {
	return &CliDeviceHandler{
		path: path,
		in:   in,
		out:  out,
	}
}

func (h *CliDeviceHandler) HandleDevice(dev Device) {
	scanner := bufio.NewScanner(os.Stdin)
	if dev.Available() {
		fmt.Println("Found new device:", dev.Dev(), "name:", dev.Label())
		fmt.Println("Name?")
		scanner.Scan()
		name := scanner.Text()
		fmt.Println("Year?")
		scanner.Scan()
		year := scanner.Text()
		h.out <- Rip{
			path: h.path,
			name: name,
			year: year,
			dev:  dev,
		}
	}
}
