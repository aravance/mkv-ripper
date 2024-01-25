package drive

import (
	"log"
	"sync"

	"github.com/aravance/go-makemkv"
)

type DriveStatus string

const (
	StatusReady   DriveStatus = "Ready"
	StatusReading             = "Reading"
	StatusMkv                 = "Mkv"
	StatusEmpty               = "Empty"
)

type DriveManager interface {
	Eject() error
	GetDiscInfo() (makemkv.DiscInfo, bool)
	HasDisc() bool
	Start() error
	Stop() error
	Status() DriveStatus
}

type driveManager struct {
	discdb       DiscDatabase
	udevListener *udevListener
	mutex        sync.Mutex
	started      bool
	device       *udevDevice
	status       DriveStatus
}

func NewUdevDeviceManager(discdb DiscDatabase) DriveManager {
	m := driveManager{
		discdb: discdb,
		status: StatusEmpty,
	}
	m.udevListener = newUdevListener(m.onDevice)
	return &m
}

func (m *driveManager) Status() DriveStatus {
	return m.status
}

func (m *driveManager) GetDiscInfo() (makemkv.DiscInfo, bool) {
	if !m.HasDisc() {
		return makemkv.DiscInfo{}, false
	}
	return m.discdb.GetDiscInfo(m.device.Uuid())
}

func (m *driveManager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.started {
		return nil
	}

	m.udevListener.Start()
	m.started = true
	return nil
}

func (m *driveManager) onDevice(dev *udevDevice) {
	if m.device == nil || dev.Device() == m.device.Device() {
		if !dev.Available() {
			m.device = nil
			m.status = StatusEmpty
		} else {
			m.device = dev
			_, ok := m.discdb.GetDiscInfo(dev.Uuid())
			if !ok {
				m.status = StatusReading
				job := makemkv.Info(dev, makemkv.MkvOptions{})
				m.status = StatusReady
				if info, err := job.Run(); err != nil {
					log.Println("error running makemkv info", err)
				} else {
					m.discdb.SaveDiscInfo(dev.Uuid(), *info)
				}
			} else {
				m.status = StatusReady
			}
			// TODO notify?
		}
	}
}

func (m *driveManager) Stop() error {
	m.udevListener.Stop()
	return nil
}

func (m *driveManager) GetInfo() (*makemkv.DiscInfo, error) {
	panic("unimplemented")
}

func (m *driveManager) Eject() error {
	panic("unimplemented")
}

func (m *driveManager) HasDisc() bool {
	return m.device != nil && m.device.Available()
}
