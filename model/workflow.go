package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
)

type WorkflowStatus string

const (
	StatusDone      WorkflowStatus = "Done"
	StatusImporting WorkflowStatus = "Importing"
	StatusRipping   WorkflowStatus = "Ripping"
	StatusPending   WorkflowStatus = "Pending"
	StatusError     WorkflowStatus = "Error"
	StatusStart     WorkflowStatus = "Start"
)

type MkvFile struct {
	Filename   string
	Shasum     string
	Resolution string
}

type Workflow struct {
	DiscId       string
	TitleId      int
	Label        string
	OriginalName string
	Status       WorkflowStatus
	ImdbId       *string  `json:",omitempty"`
	Name         *string  `json:",omitempty"`
	Year         *string  `json:",omitempty"`
	File         *MkvFile `json:",omitempty"`
}

type WorkflowManager interface {
	NewWorkflow(discId string, titleId int, label string, name string) (*Workflow, bool)
	GetWorkflow(discId string, titleId int) *Workflow
	GetWorkflows() []*Workflow
	Save(*Workflow) error
	Clean(*Workflow) error
}

type workflowManager struct {
	workflows map[string]*Workflow
	file      string
}

func newWorkflow(discId string, titleId int, label string, name string) *Workflow {
	return &Workflow{
		DiscId:       discId,
		TitleId:      titleId,
		Label:        label,
		OriginalName: name,
		Status:       StatusStart,
		Name:         nil,
		Year:         nil,
		File:         nil,
	}
}

func NewJsonWorkflowManager(file string) WorkflowManager {
	workflows, err := loadWorkflowJson(file)
	if err != nil {
		workflows = make(map[string]*Workflow)
	}
	m := workflowManager{
		workflows,
		file,
	}
	return &m
}

func id(discId string, titleId int) string {
	return fmt.Sprintf("%s-%d", discId, titleId)
}

// TODO kill this
func (w *Workflow) Id() string {
	return id(w.DiscId, w.TitleId)
}

func (m *workflowManager) NewWorkflow(discId string, titleId int, label string, name string) (*Workflow, bool) {
	w, containsKey := m.workflows[id(discId, titleId)]
	if containsKey {
		w.Label = label
		return w, false
	}
	w = newWorkflow(discId, titleId, label, name)
	return w, true
}

func (m *workflowManager) GetWorkflow(discId string, titleId int) *Workflow {
	return m.workflows[id(discId, titleId)]
}

func (m *workflowManager) GetWorkflows() []*Workflow {
	values := make([]*Workflow, 0, len(m.workflows))
	for _, v := range m.workflows {
		values = append(values, v)
	}
	return values
}

func (m *workflowManager) Save(w *Workflow) error {
	m.workflows[id(w.DiscId, w.TitleId)] = w

	if bytes, err := json.Marshal(m.workflows); err != nil {
		return err
	} else if err := os.WriteFile(m.file, bytes, 0644); err != nil {
		return err
	} else {
		return nil
	}
}

func (m *workflowManager) Clean(w *Workflow) error {
	err := os.Remove(w.File.Filename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Println("error removing file", w.File.Filename)
		return err
	}
	w.File = nil
	return m.Save(w)
}

func loadWorkflowJson(file string) (map[string]*Workflow, error) {
	var out map[string]*Workflow
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
