package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"strings"
)

type MkvFile struct {
	Filename   string `json:"filename"`
	Shasum     string `json:"shasum"`
	Resolution string `json:"resolution"`
}

type Workflow struct {
	Id    string    `json:"-"`
	Label string    `json:"label"`
	Name  *string   `json:"name,omitempty"`
	Year  *string   `json:"year,omitempty"`
	Files []MkvFile `json:"files"`
	dir   string    `json:"-"`
}

var dir = "."

func SetDir(d string) {
	dir = d
}

func LoadExistingWorkflows() []*Workflow {
	files, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = os.MkdirAll(dir, 0775)
			return []*Workflow{}
		}
		log.Fatal(err)
	}

	result := []*Workflow{}
	for _, file := range files {
		ext := path.Ext(file.Name())
		if ext == ".json" {
			log.Println("Found existing file:", file)
			id := strings.TrimSuffix(file.Name(), path.Ext(file.Name()))
			workflow, err := LoadWorkflow(id)
			if err != nil {
				log.Println("Failed to load workflow:", file, err)
				continue
			}

			result = append(result, workflow)
		}
	}
	return result
}

func NewWorkflow(id string, dir string, label string) *Workflow {
	return &Workflow{
		Id:    id,
		Label: label,
		Name:  nil,
		Year:  nil,
		Files: make([]MkvFile, 0),
		dir:   dir,
	}
}

func LoadWorkflow(id string) (*Workflow, error) {
	file := path.Join(dir, id+".json")
	w := NewWorkflow(id, dir, "")

	bytes, err := os.ReadFile(file)
	if err != nil {
		log.Println("Failed to read file:", file, err)
		return nil, err
	}

	err = json.Unmarshal(bytes, w)
	if err != nil {
		log.Println("Failed to unmarshal json:", file, err)
		return nil, err
	}

	return w, nil
}

func (t *Workflow) Save() error {
	file := path.Join(t.dir, fmt.Sprintf("%s.json", t.Id))
	if bytes, err := json.Marshal(*t); err != nil {
		return err
	} else if err := os.WriteFile(file, bytes, 0664); err != nil {
		return err
	} else {
		return nil
	}
}

func (t *Workflow) JsonFile() string {
	return path.Join(t.dir, fmt.Sprintf("%s.json", t.Id))
}

func (t *Workflow) AddFiles(mkvFiles ...MkvFile) {
	t.Files = append(t.Files, mkvFiles...)
}

func (t *Workflow) AddMovieDetails(name string, year string) {
	t.Name = &name
	t.Year = &year
}
