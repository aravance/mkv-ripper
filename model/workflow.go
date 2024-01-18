package model

import (
	"encoding/json"
	"errors"
	"log"
	"os"
)

type WorkflowStatus string

const (
	StatusDone      WorkflowStatus = "Done"
	StatusImporting WorkflowStatus = "Importing"
	StatusRipping   WorkflowStatus = "Ripping"
	StatusPending   WorkflowStatus = "Pending"
	StatusStart     WorkflowStatus = "Start"
)

type MkvFile struct {
	Filename   string
	Shasum     string
	Resolution string
}

type Workflow struct {
	Id     string
	Label  string
	Status WorkflowStatus
	Name   *string  `json:",omitempty"`
	Year   *string  `json:",omitempty"`
	File   *MkvFile `json:",omitempty"`
}

type WorkflowManager interface {
	NewWorkflow(id string, label string) (*Workflow, bool)
	GetWorkflow(id string) *Workflow
	GetWorkflows() []*Workflow
	Save(*Workflow) error
	Clean(*Workflow) error
}

type workflowManager struct {
	workflows map[string]*Workflow
	file      string
}

func newWorkflow(id string, label string) *Workflow {
	return &Workflow{
		Id:     id,
		Label:  label,
		Status: StatusStart,
		Name:   nil,
		Year:   nil,
		File:   nil,
	}
}

func NewWorkflowManager(file string) WorkflowManager {
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

func (m *workflowManager) NewWorkflow(id string, label string) (*Workflow, bool) {
	w, containsKey := m.workflows[id]
	if containsKey {
		return w, false
	}
	w = newWorkflow(id, label)
	return w, true
}

func (m *workflowManager) GetWorkflow(id string) *Workflow {
	return m.workflows[id]
}

func (m *workflowManager) GetWorkflows() []*Workflow {
	values := make([]*Workflow, 0, len(m.workflows))
	for _, v := range m.workflows {
		values = append(values, v)
	}
	return values
}

func (m *workflowManager) Save(w *Workflow) error {
	m.workflows[w.Id] = w

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
