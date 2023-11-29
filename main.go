package main

import (
	"log"
	"os"
	"os/signal"
)

const defaultPath = "/var/rip"

type MovieDetails struct {
	name    string
	year    string
	variant string
}

func main() {
	logfile, err := os.OpenFile("mkv.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		log.Fatalln("Failed to open log file", err)
	}
	defer logfile.Close()
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
