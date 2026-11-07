package model

import (
	"time"

	"github.com/aravance/go-makemkv"
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
	Resolution   *string  `json:",omitempty"`
	// Collections are the media-server collections (by name) the movie
	// should be added to once ingested.
	Collections []string  `json:",omitempty"`
	UpdatedAt   time.Time `json:",omitempty"`

	MkvStatus *makemkv.Status `json:"-"`
}

// MovieGroup groups workflows for the same movie (e.g. separate 4k and 1080p
// disc rips) into a single history entry with one badge per resolution.
type MovieGroup struct {
	Name      string
	Year      string
	ImdbId    *string
	Poster    string
	UpdatedAt time.Time
	Workflows []*Workflow
}
