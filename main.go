package main

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
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
			dir, err := os.MkdirTemp(defaultPath, "rip")
			if err != nil {
				log.Println("Failed to make temp directory")
			} else {
				log.Println("Created", dir)
				statchan, _ := ripDevice(dir, rip.name, rip.dev.Device(), rip.dev.Type())
				for stat := range statchan {
					fmt.Println(stat)
				}
				log.Println("Done")
				files, err := ioutil.ReadDir(dir)
				if err != nil {
					log.Println("Error opening dir", dir)
				} else {
					for _, file := range files {
						name := fmt.Sprintf("%s (%s)", rip.name, rip.year)
						oldfile := filepath.Join(dir, file.Name())
						newdir := filepath.Join(defaultPath, name)
						newfile := filepath.Join(newdir, name+".mkv")
						os.Mkdir(newdir, 0775)
						os.Rename(oldfile, newfile)

						shasum, _ := shasum(newfile)
						shafile := filepath.Join(defaultPath, "Movies.sha256")
						path := fmt.Sprintf("Movies/%s/%s.mkv", name, name)
						addShasum(shafile, shasum, path)

						log.Printf("%s  Movies/%s/%s.mkv\n", shasum, name, name)
					}
				}
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

func shasum(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	io.Copy(h, f)
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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
