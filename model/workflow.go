package model

import (
	"fmt"
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

// TODO kill this
func (w *Workflow) Id() string {
	return id(w.DiscId, w.TitleId)
}

func id(discId string, titleId int) string {
	return fmt.Sprintf("%s-%d", discId, titleId)
}
