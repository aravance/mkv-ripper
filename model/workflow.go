package model

import (
	"encoding/json"
	"log"
	"os"
)

type MkvFile struct {
	Filename   string
	Shasum     string
	Resolution string
}

type Workflow struct {
	Id    string
	Label string
	Name  *string `json:",omitempty"`
	Year  *string `json:",omitempty"`
	Files []MkvFile
}

type WorkflowManager interface {
	NewWorkflow(id string, label string) *Workflow
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
		Id:    id,
		Label: label,
		Name:  nil,
		Year:  nil,
		Files: make([]MkvFile, 0),
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

func (m *workflowManager) NewWorkflow(id string, label string) *Workflow {
	w, containsKey := m.workflows[id]
	if containsKey {
		// TODO throw an error?
		return w
	}
	w = newWorkflow(id, label)
	m.workflows[id] = w
	m.Save(w)
	return w
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
	if bytes, err := json.Marshal(m.workflows); err != nil {
		return err
	} else if err := os.WriteFile(m.file, bytes, 0644); err != nil {
		return err
	} else {
		return nil
	}
}

func (m *workflowManager) Clean(w *Workflow) error {
	for _, file := range w.Files {
		os.Remove(file.Filename)
	}
	return nil
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
