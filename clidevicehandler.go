package main

import (
	"bufio"
	"fmt"
	"os"
)

type CliDeviceHandler struct {
}

func NewCliDeviceHandler() *CliDeviceHandler {
	return &CliDeviceHandler{}
}

func (h *CliDeviceHandler) HandleDevice(device Device) (details *MovieDetails) {
	scanner := bufio.NewScanner(os.Stdin)
	if device.Available() {
		fmt.Println("Found new device:", device.Device(), "name:", device.Label())
		fmt.Println("Name?")
		scanner.Scan()
		name := scanner.Text()
		fmt.Println("Year?")
		scanner.Scan()
		year := scanner.Text()
		return &MovieDetails{name, year}
	}
	return nil
}
