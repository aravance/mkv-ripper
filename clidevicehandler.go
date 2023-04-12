package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

type CliDeviceHandler struct {
	in      <-chan Device
	out     chan<- RipRequest
	mutex   sync.Mutex
	started bool
}

func NewCliDeviceHandler(in <-chan Device, out chan<- RipRequest) *CliDeviceHandler {
	return &CliDeviceHandler{
		in:      in,
		out:     out,
		mutex:   sync.Mutex{},
		started: false,
	}
}

func (h *CliDeviceHandler) Start() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.started {
		return
	}
	h.started = true
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for dev := range h.in {
			if dev.Available() {
				fmt.Println("Found new device:", dev.Device(), "name:", dev.Label())
				fmt.Println("Name?")
				scanner.Scan()
				name := scanner.Text()
				fmt.Println("Year?")
				scanner.Scan()
				year := scanner.Text()
				h.out <- RipRequest{
					name: name,
					year: year,
					dev:  dev,
				}
			}
		}

	}()
}

func (h *CliDeviceHandler) Stop() {
}

func handleDevice(dev Device) *RipRequest {
	return nil
}
