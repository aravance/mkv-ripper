package main

import (
	"context"
	"fmt"
	"sync"

	udev "github.com/farjump/go-libudev"
)

type UdevDevice struct {
	label string
	dev   string
	udev  *udev.Device
}

func (t *UdevDevice) Label() string {
	return t.label
}

func (t *UdevDevice) Dev() string {
	return t.dev
}

func (t *UdevDevice) Type() string {
	return "dev"
}

func (t *UdevDevice) Available() bool {
	return t.udev != nil
}

type UdevListener struct {
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	devices map[string]UdevDevice
	channel chan Device
}

func NewUdevListener(devchan chan Device) *UdevListener {
	return &UdevListener{
		devices: make(map[string]UdevDevice),
		channel: devchan,
	}
}

func (t *UdevListener) Start() error {
	u := udev.Udev{}
	m := u.NewMonitorFromNetlink("udev")

	m.FilterAddMatchSubsystemDevtype("block", "disk")
	m.FilterAddMatchTag("systemd")

	var ctx context.Context
	ctx, t.cancel = context.WithCancel(context.Background())

	t.wg = sync.WaitGroup{}
	t.wg.Add(1)
	defer t.wg.Done()

	devchan, err := m.DeviceChan(ctx)
	if err == nil {
		fmt.Println("Udev channel opened")
		for d := range devchan {
			if d.PropertyValue("ID_CDROM_MEDIA") == "1" {
				fmt.Println("Found media:", d.Sysname(), "name:", d.PropertyValue("ID_FS_LABEL"))
				dev := UdevDevice{
					label: d.PropertyValue("ID_FS_LABEL"),
					dev:   d.PropertyValue("DEVNAME"),
					udev:  d,
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
		fmt.Println("Udev channel closed")
	} else {
		fmt.Println("Error opening channel:", err)
	}

	return err
}

func (t *UdevListener) Stop() {
	t.cancel()
	t.wg.Wait()
}
