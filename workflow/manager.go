package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path"

	"github.com/aravance/go-makemkv"
	"github.com/aravance/mkv-ripper/drive"
	"github.com/aravance/mkv-ripper/ingest"
	"github.com/aravance/mkv-ripper/model"
)

type WorkflowManager interface {
	Start(*model.Workflow) error
	Ingest(*model.Workflow) error
	NewWorkflow(discId string, titleId int, label string, name string) (*model.Workflow, bool)
	GetWorkflow(discId string, titleId int) *model.Workflow
	GetWorkflows(discId string) []*model.Workflow
	GetAllWorkflows() []*model.Workflow
	Save(*model.Workflow) error
	Clean(*model.Workflow) error
}

func (m *workflowManager) Start(wf *model.Workflow) error {
	if wf.Status == model.StatusImporting || wf.Status == model.StatusRipping {
		return fmt.Errorf("workflow is already running: %s", wf.Status)
	}
	disc := m.driveman.GetDisc()
	if disc == nil {
		return fmt.Errorf("disc cannot be nil")
	}
	di, ok := m.discdb.GetDiscInfo(disc.Uuid)
	if !ok || di == nil {
		return fmt.Errorf("info cannot be nil")
	}
	if wf.TitleId >= len(di.Titles) {
		return fmt.Errorf("title cannot be nil")
	}
	ti := &di.Titles[wf.TitleId]

	wf.Status = model.StatusRipping
	m.Save(wf)

	dir := path.Join(m.outdir, wf.DiscId)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Println("error making dir:", dir, "err:", err)
		wf.Status = model.StatusError
		m.Save(wf)
		return err
	}

	statchan := make(chan makemkv.Status)
	defer close(statchan)

	go func() {
		for stat := range statchan {
			disc.MkvStatus = &stat
		}
	}()

	f, err := m.driveman.RipFile(ti, dir, statchan)
	if err != nil {
		log.Println("error ripping:", wf, "err:", err)
		wf.Status = model.StatusError
		m.Save(wf)
		return err
	}

	wf.File = f
	wf.Status = model.StatusPending
	m.Save(wf)

	go m.Ingest(wf)

	return nil
}

func (m *workflowManager) Ingest(wf *model.Workflow) error {
	if wf.Status != model.StatusPending {
		log.Println("ingest workflow not ready", wf)
		return fmt.Errorf("workflow not ready, status: %s", wf.Status)
	}
	log.Println("ingesting", wf)

	file := wf.File
	if file == nil {
		log.Println("no files to ingest")
		return fmt.Errorf("no files to ingest")
	}

	if wf.Name == nil || wf.Year == nil {
		log.Println("name or year is not set")
		return fmt.Errorf("name or year is not set")
	}

	wf.Status = model.StatusImporting
	m.Save(wf)

	var err error
	for _, target := range m.targets {
		ingester, err := ingest.NewIngester(target)
		if err != nil {
			log.Println("error finding ingester", err, "for target", target)
			continue
		}

		err = ingester.Ingest(*file, *wf.Name, *wf.Year)
		if err != nil {
			log.Println("error running ingester", ingester, err)
		}
	}

	if err == nil {
		log.Println("cleaning workflow")
		m.Clean(wf)
		wf.Status = model.StatusDone
		m.Save(wf)
	}
	return nil
}

type workflowManager struct {
	workflows map[string]map[int]*model.Workflow
	driveman  drive.DriveManager
	discdb    drive.DiscDatabase
	targets   []*url.URL
	outdir    string
	file      string
}

func newWorkflow(discId string, titleId int, label string, name string) *model.Workflow {
	return &model.Workflow{
		DiscId:       discId,
		TitleId:      titleId,
		Label:        label,
		OriginalName: name,
		Status:       model.StatusStart,
		Name:         nil,
		Year:         nil,
		File:         nil,
	}
}

func NewJsonWorkflowManager(driveman drive.DriveManager, discdb drive.DiscDatabase, targets []*url.URL, outdir string, file string) WorkflowManager {
	workflows, err := loadWorkflowJson(file)
	if err != nil {
		workflows = make(map[string]map[int]*model.Workflow)
	}
	m := workflowManager{
		workflows: workflows,
		driveman:  driveman,
		discdb:    discdb,
		targets:   targets,
		outdir:    outdir,
		file:      file,
	}
	return &m
}

func getOrCreate(wfs map[string]map[int]*model.Workflow, key string) map[int]*model.Workflow {
	w, containsKey := wfs[key]
	if !containsKey {
		w = make(map[int]*model.Workflow)
		wfs[key] = w
	}
	return w
}

func (m *workflowManager) NewWorkflow(discId string, titleId int, label string, name string) (*model.Workflow, bool) {
	titleWfs := getOrCreate(m.workflows, discId)
	w, containsKey := titleWfs[titleId]
	if containsKey {
		w.Label = label
		return w, false
	}
	w = newWorkflow(discId, titleId, label, name)
	return w, true
}

func (m *workflowManager) GetWorkflow(discId string, titleId int) *model.Workflow {
	titleWfs, containsKey := m.workflows[discId]
	if !containsKey {
		return nil
	}
	return titleWfs[titleId]
}

func (m *workflowManager) GetWorkflows(discId string) []*model.Workflow {
	titleWfs, containsKey := m.workflows[discId]
	if !containsKey {
		return make([]*model.Workflow, 0)
	}

	values := make([]*model.Workflow, 0, len(titleWfs))
	for _, v := range titleWfs {
		values = append(values, v)
	}
	return values
}

func (m *workflowManager) GetAllWorkflows() []*model.Workflow {
	values := make([]*model.Workflow, 0, len(m.workflows))
	for _, t := range m.workflows {
		for _, v := range t {
			values = append(values, v)
		}
	}
	return values
}

func (m *workflowManager) Save(w *model.Workflow) error {
	titleWfs := getOrCreate(m.workflows, w.DiscId)
	titleWfs[w.TitleId] = w

	if bytes, err := json.Marshal(m.workflows); err != nil {
		return err
	} else if err := os.WriteFile(m.file, bytes, 0644); err != nil {
		return err
	} else {
		return nil
	}
}

func (m *workflowManager) Clean(w *model.Workflow) error {
	// attempt to remove the rip dir, but ignore failures for non-empty
	os.Remove(path.Join(m.outdir, w.DiscId))

	err := os.Remove(w.File.Filename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Println("error removing file", w.File.Filename)
		return err
	}
	w.File = nil
	return m.Save(w)
}

func loadWorkflowJson(file string) (map[string]map[int]*model.Workflow, error) {
	var out map[string]map[int]*model.Workflow
	bytes, err := os.ReadFile(file)
	if err != nil {
		log.Println("Failed to read file:", file, err)
		return nil, err
	}

	err = json.Unmarshal(bytes, &out)
	if err != nil {
		log.Println("failed to unmarshal json:", file, err)
		return nil, err
	}

	return out, nil
}
