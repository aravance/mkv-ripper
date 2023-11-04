package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
)

const defaultPath = "/var/rip"

type RipRequest struct {
	path string
	name string
	year string
	dev  Device
}

type MovieDetails struct {
	name string
	year string
}

func handleDevice(device Device) *MovieDetails {
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

func main() {
	logfile, err := os.OpenFile("mkv.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		log.Fatalln("Failed to open log file", err)
	}
	log.SetOutput(logfile)

	devchan := make(chan Device)

	listener := NewUdevListener(devchan)
	listener.Start()
	defer listener.Stop()

	handler := NewCliDeviceHandler()
	path := defaultPath

	go func() {
		for dev := range devchan {
			if dev.Available() {
				go func(device Device) {
					workflow := NewWorkflow(handler, device, path)
					workflow.Start()
				}(dev)
			} else {
				log.Println("Unavailable device", dev)
			}
		}
	}()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan

	log.Println("Shutting down")
	close(devchan)
}

func readSums(file string) map[string]string {
	m := make(map[string]string)
	f, _ := os.Open(file)
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "  ")
		sum := parts[0]
		movie := parts[1]
		m[movie] = sum
	}

	return m
}

func addShasum(file string, shasum string, name string) {
	sums := readSums(file)
	sums[name] = shasum
	f, _ := os.OpenFile(file, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0664)
	defer f.Close()

	keys := make([]string, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		f.WriteString(fmt.Sprintf("%s  %s\n", sums[key], key))
	}
}
