package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
)

const defaultPath = "/var/rip"

type Device interface {
	Label() string
	Device() string
	Type() string
	Available() bool
}

type RipRequest struct {
	path string
	name string
	year string
	dev  Device
}

func main() {
	logfile, err := os.OpenFile("mkv.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		log.Fatalln("Failed to open log file", err)
	}
	log.SetOutput(logfile)

	devchan := make(chan Device, 10)
	ripchan := make(chan RipRequest, 10)

	listener := NewUdevListener(devchan)
	listener.Start()
	defer listener.Stop()

	handler := NewCliDeviceHandler(devchan, ripchan)
	handler.Start()
	defer handler.Stop()

	go func() {
		for rip := range ripchan {
			log.Printf("Rip: %+v\n", rip)
			dir, err := os.MkdirTemp(defaultPath, rip.name)
			if err != nil {
				log.Println("Failed to make temp directory")
			} else {
				log.Println("Created", dir)
				statchan, _ := ripDevice(dir, rip.name, rip.dev.Device(), rip.dev.Type())
				for stat := range statchan {
					fmt.Println(stat)
				}
				log.Println("Done")
				os.RemoveAll(dir)
				log.Println("Deleted", dir)
			}
		}
	}()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan

	log.Println("Shutting down")
	close(devchan)
	close(ripchan)
}
