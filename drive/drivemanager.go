package drive

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/model"
	"github.com/aravance/mkv-ripper/util"
)

type DriveStatus string

const (
	StatusReady   DriveStatus = "Ready"
	StatusReading             = "Reading"
	StatusMkv                 = "Mkv"
	StatusEmpty               = "Empty"
)

type Disc struct {
	Label string
	Uuid  string
}

type DriveManager interface {
	Eject() error
	GetDiscInfo() (*makemkv.DiscInfo, error)
	GetDisc() *Disc
	HasDisc() bool
	Start() error
	Stop() error
	Status() DriveStatus
	RipFile(title *makemkv.TitleInfo, outdir string, outchan chan makemkv.Status) (*model.MkvFile, error)
}

func NewUdevDriveManager(onDisc func(*Disc)) DriveManager {
	m := driveManager{
		status: StatusEmpty,
		onDisc: onDisc,
	}
	m.udevListener = newUdevListener(m.onDevice)
	return &m
}

type driveManager struct {
	udevListener *udevListener
	mutex        sync.Mutex
	started      bool
	device       *udevDevice
	status       DriveStatus
	disc         *Disc
	onDisc       func(*Disc)
}

func (m *driveManager) Status() DriveStatus {
	return m.status
}

func (m *driveManager) GetDisc() *Disc {
	return m.disc
}

func (m *driveManager) GetDiscInfo() (*makemkv.DiscInfo, error) {
	if !m.HasDisc() {
		return nil, fmt.Errorf("no disc available")
	}

	if m.Status() != StatusReady {
		return nil, fmt.Errorf("drive is busy")
	}

	if err := m.setBusy(StatusReading); err != nil {
		return nil, err
	}
	defer m.setIdle()

	job := makemkv.Info(m.device, makemkv.MkvOptions{})
	if info, err := job.Run(); err != nil {
		log.Println("error running makemkv info", err)
		return info, err
	} else {
		return info, nil
	}
}

func (m *driveManager) RipFile(title *makemkv.TitleInfo, outdir string, statchan chan makemkv.Status) (*model.MkvFile, error) {
	if m.device == nil || !m.device.Available() {
		return nil, fmt.Errorf("no device available")
	}

	if m.Status() != StatusReady {
		return nil, fmt.Errorf("drive is busy")
	}

	if err := m.setBusy(StatusMkv); err != nil {
		return nil, err
	}
	defer m.setIdle()

	ripdir, err := os.MkdirTemp(outdir, ".rip")
	if err != nil {
		log.Println("failed to make temp dir", err)
		return nil, err
	}
	defer os.RemoveAll(ripdir)

	opts := makemkv.MkvOptions{
		Progress:  makemkv.Stropt("-same"),
		Minlength: makemkv.Intopt(3600),
		Noscan:    true,
	}
	log.Println("starting makemkv")
	mkvjob := makemkv.Mkv(m.device, title.Id, ripdir, opts)
	mkvjob.Statuschan = statchan

	if err := mkvjob.Run(); err != nil {
		log.Println("error ripping device", err)
		return nil, err
	}

	oldfile := path.Join(ripdir, title.FileName)
	log.Println("starting sha256sum for " + oldfile)
	shasum, err := util.Sha256sum(oldfile)
	if err != nil {
		log.Println("error in sha256sum for " + oldfile)
		return nil, err
	} else {
		log.Println("sha256sum " + title.FileName + ": " + shasum)
	}

	newfile := path.Join(outdir, title.FileName)
	os.Rename(oldfile, newfile)

	resolution := "unknown"
	if len(title.VideoStreams) > 0 {
		_, height, ok := strings.Cut(title.VideoStreams[0].VideoSize, "x")
		if ok {
			switch height {
			case "2160":
				resolution = "4k"
			default:
				resolution = fmt.Sprintf("%sp", height)
			}
		}
	}

	return &model.MkvFile{
		Filename:   newfile,
		Shasum:     shasum,
		Resolution: resolution,
	}, nil
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
	if !dev.Available() {
		m.device = nil
		m.disc = nil
	} else {
		m.device = dev
		m.disc = &Disc{
			Label: dev.Label(),
			Uuid:  dev.Uuid(),
		}
	}
	m.setIdle()
	go m.onDisc(m.disc)
}

func (m *driveManager) Stop() error {
	m.udevListener.Stop()
	return nil
}

func (m *driveManager) Eject() error {
	panic("unimplemented")
}

func (m *driveManager) HasDisc() bool {
	return m.device != nil && m.device.Available()
}

func (m *driveManager) setBusy(s DriveStatus) error {
	switch m.status {
	case StatusReady:
		m.status = s
		return nil
	case StatusEmpty:
		return fmt.Errorf("drive is empty")
	case StatusMkv:
		fallthrough
	case StatusReading:
		return fmt.Errorf("drive is busy")
	default:
		return fmt.Errorf("unknown drive status")
	}
}

func (m *driveManager) setIdle() {
	if m.device != nil && m.device.Available() {
		m.status = StatusReady
	} else {
		m.status = StatusEmpty
	}
}
