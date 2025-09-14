package model

import "github.com/jfrog/build-info-go/entities"

type GitLogEntryView struct {
	Data     []GitLogEntry     `json:"data"`
	Link     string            `json:"link"`
	Artifact entities.Artifact `json:"artifact"`
}
