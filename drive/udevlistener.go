package drive

import (
	"context"
	"log"
	"sync"

	udev "github.com/farjump/go-libudev"
)

type udevDevice struct {
	udev *udev.Device
}

func (t *udevDevice) Label() string {
	if t.udev == nil {
		return ""
	} else {
		return t.udev.PropertyValue("ID_FS_LABEL")
	}
}

func (t *udevDevice) Uuid() string {
	if t.udev == nil {
		return ""
	} else {
		return t.udev.PropertyValue("ID_FS_UUID")
	}
}

func (t *udevDevice) Device() string {
	if t.udev == nil {
		return ""
	} else {
		return t.udev.PropertyValue("DEVNAME")
	}
}

func (t *udevDevice) Type() string {
	return "dev"
}

func (t *udevDevice) Available() bool {
	return t.udev != nil && t.udev.PropertyValue("SYSTEMD_READY") != "0"
}

type udevListener struct {
	devices map[string]udevDevice
	notify  func(*udevDevice)
	started bool
	stopped bool
	wg      sync.WaitGroup
	mutex   sync.Mutex
	cancel  context.CancelFunc
}

func newUdevListener(notify func(*udevDevice)) *udevListener {
	return &udevListener{
		devices: make(map[string]udevDevice),
		notify:  notify,
		started: false,
		wg:      sync.WaitGroup{},
		mutex:   sync.Mutex{},
	}
}

func (t *udevListener) Start() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.started || t.stopped {
		return
	}

	t.started = true
	go t.run()
}

func (t *udevListener) run() {
	t.wg.Add(1)
	defer t.wg.Done()

	u := udev.Udev{}
	m := u.NewMonitorFromNetlink("udev")

	m.FilterAddMatchSubsystemDevtype("block", "disk")
	m.FilterAddMatchTag("systemd")

	var ctx context.Context
	ctx, t.cancel = context.WithCancel(context.Background())

	devchan, err := m.DeviceChan(ctx)
	if err == nil {
		log.Println("udev channel opened")
		for d := range devchan {
			if d.PropertyValue("ID_CDROM_MEDIA") == "1" {
				log.Println("found media:", d.Sysname(), "name:", d.PropertyValue("ID_FS_LABEL"))
				dev := udevDevice{
					udev: d,
				}
				t.devices[d.PropertyValue("DEVNAME")] = dev
				go t.notify(&dev)
			} else if d.PropertyValue("SYSTEMD_READY") == "0" {
				dev, ok := t.devices[d.PropertyValue("DEVNAME")]
				if ok {
					delete(t.devices, d.PropertyValue("DEVNAME"))
					dev.udev = nil
					go t.notify(&dev)
				}
			}
		}
		log.Println("udev channel closed")
	} else {
		log.Println("error opening udev channel:", err)
	}
}

func (t *udevListener) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.stopped = true
	if t.cancel != nil {
		t.cancel()
	}
	t.wg.Wait()
}
