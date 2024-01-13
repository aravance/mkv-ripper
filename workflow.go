package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
)

type MkvFile struct {
	Filename   string `json:"filename"`
	Original   string `json:"original"`
	Shasum     string `json:"shasum"`
	Resolution string `json:"resolution"`
}

type Workflow struct {
	Id    string `json:"-"`
	Dir   string `json:"-"`
	Label string  `json:"label"`
	Name  *string `json:"name,omitempty"`
	Year  *string `json:"year,omitempty"`
	Files []MkvFile `json:"files"`
}

func NewWorkflow(id string, dir string, label string) *Workflow {
	return &Workflow{
		Id:    id,
		Dir:   dir,
		Label: label,
		Name:  nil,
		Year:  nil,
		Files: make([]MkvFile, 0),
	}
}

func LoadWorkflow(file string) (*Workflow, error) {
	ext := path.Ext(file)
	if ext != ".json" {
		return nil, fmt.Errorf("Workflow file must be json")
	}

	dir, f := path.Split(file)
	id := strings.TrimSuffix(f, ext)
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
	file := path.Join(t.Dir, fmt.Sprintf("%s.json", t.Id))
	if bytes, err := json.Marshal(*t); err != nil {
		return err
	} else if err := os.WriteFile(file, bytes, 0664); err != nil {
		return err
	} else {
		return nil
	}
}

func (t *Workflow) JsonFile() string {
	return path.Join(t.Dir, fmt.Sprintf("%s.json", t.Id))
}

func (t *Workflow) AddFiles(mkvFiles ...MkvFile) {
	t.Files = append(t.Files, mkvFiles...)
}

func (t *Workflow) AddMovieDetails(movieDetails MovieDetails) {
	t.Name = &movieDetails.name
	t.Year = &movieDetails.year
}
