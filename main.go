package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"

	udev "github.com/farjump/go-libudev"
)

func listen(wg *sync.WaitGroup) context.CancelFunc {
	u := udev.Udev{}
	m := u.NewMonitorFromNetlink("udev")

	m.FilterAddMatchSubsystemDevtype("block", "disk")
	m.FilterAddMatchTag("systemd")

	fmt.Println("Started listening on udev channel")
	ctx, cancel := context.WithCancel(context.Background())
	devchan, _ := m.DeviceChan(ctx)

	go func() {
		for d := range devchan {
			if d.Properties()["ID_CDROM_MEDIA"] == "1" {
				fmt.Println("Found media:", d.Properties()["ID_FS_LABEL"])
			} else if d.Properties()["SYSTEMD_READY"] == "0" {
				fmt.Println("Disk unmounted")
			}
		}
		fmt.Println("Udev channel closed")
		wg.Done()
	}()

	return cancel
}

func waitForShutdown() {
	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan

	fmt.Println("Shutting down")
}

func main() {
	var wg sync.WaitGroup
	wg.Add(1)

	cancel := listen(&wg)

	waitForShutdown()
	cancel()
	wg.Wait()
}
