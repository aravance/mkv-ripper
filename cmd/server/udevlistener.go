package main

import (
	"context"
	"log"
	"sync"

	udev "github.com/farjump/go-libudev"
)

type UdevDevice struct {
	udev *udev.Device
}

func (t *UdevDevice) Label() string {
	if t.udev == nil {
		return ""
	} else {
		return t.udev.PropertyValue("ID_FS_LABEL")
	}
}

func (t *UdevDevice) Device() string {
	if t.udev == nil {
		return ""
	} else {
		return t.udev.PropertyValue("DEVNAME")
	}
}

func (t *UdevDevice) Type() string {
	return "dev"
}

func (t *UdevDevice) Available() bool {
	return t.udev != nil && t.udev.PropertyValue("SYSTEMD_READY") != "0"
}

type UdevListener struct {
	devices map[string]UdevDevice
	channel chan<- *UdevDevice
	started bool
	wg      sync.WaitGroup
	mutex   sync.Mutex
	cancel  context.CancelFunc
}

func NewUdevListener(devchan chan *UdevDevice) *UdevListener {
	return &UdevListener{
		devices: make(map[string]UdevDevice),
		channel: devchan,
		started: false,
		wg:      sync.WaitGroup{},
		mutex:   sync.Mutex{},
	}
}

func (t *UdevListener) Start() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.started {
		return
	}
	t.started = true
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()

		u := udev.Udev{}
		m := u.NewMonitorFromNetlink("udev")

		m.FilterAddMatchSubsystemDevtype("block", "disk")
		m.FilterAddMatchTag("systemd")

		var ctx context.Context
		ctx, t.cancel = context.WithCancel(context.Background())

		devchan, err := m.DeviceChan(ctx)
		if err == nil {
			log.Println("Udev channel opened")
			for d := range devchan {
				if d.PropertyValue("ID_CDROM_MEDIA") == "1" {
					log.Println("Found media:", d.Sysname(), "name:", d.PropertyValue("ID_FS_LABEL"))
					dev := UdevDevice{
						udev: d,
					}
					t.devices[d.PropertyValue("DEVNAME")] = dev
					t.channel <- &dev
				} else if d.PropertyValue("SYSTEMD_READY") == "0" {
					dev, ok := t.devices[d.PropertyValue("DEVNAME")]
					if ok {
						delete(t.devices, d.PropertyValue("DEVNAME"))
						dev.udev = nil
						t.channel <- &dev
					}
				}
			}
			log.Println("Udev channel closed")
		} else {
			log.Println("Error opening channel:", err)
		}
	}()
}

func (t *UdevListener) Stop() {
	t.cancel()
	t.wg.Wait()
}
