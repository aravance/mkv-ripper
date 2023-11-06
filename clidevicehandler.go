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
		variant := ""
		fmt.Println("Variant? [1] 4k  [2] 1080p  [3] 720p")
		scanner.Scan()
		switch scanner.Text() {
		case "1":
			variant = "4k"
		case "2":
			variant = "1080p"
		case "3":
			variant = "720p"
		default:
			variant = "4k"
		}
		return &MovieDetails{name, year, variant}
	}
	return nil
}
