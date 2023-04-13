package main

import (
	"bufio"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

type RipStatus struct {
	title   string
	channel string
	current int
	max     int
}

func ripDevice(path string, name string, device string, deviceType string) (chan RipStatus, error) {
	dev := deviceType + ":" + device
	cmd := exec.Command("makemkvcon", "-r", "--noscan", "--progress=-same", "mkv", dev, "all", path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		log.Println("Failed to call makemkvcon")
		return nil, err
	}
	statuschan := make(chan RipStatus)
	go func() {
		log.Println("Ripping", name)
		scanner := bufio.NewScanner(out)
		cmd.Start()

		var title string
		var channel string
		var current int
		var max int
		for scanner.Scan() {
			line := scanner.Text()
			log.Println(line)
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
				statuschan <- RipStatus{
					title:   title,
					channel: channel,
					current: current,
					max:     max,
				}
			}
		}
		log.Println("Finished ripping", name)
		close(statuschan)
	}()
	return statuschan, nil
}
