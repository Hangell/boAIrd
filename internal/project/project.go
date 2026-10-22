// Package project defines provider-independent project state.
package project

import (
	"errors"
	"fmt"
	"strings"
)

type Status string

const (
	StatusCreated     Status = "CREATED"
	StatusConfiguring Status = "CONFIGURING"
	StatusReady       Status = "READY"
	StatusRunning     Status = "RUNNING"
	StatusPaused      Status = "PAUSED"
	StatusArchived    Status = "ARCHIVED"
)

var ErrInvalidTransition = errors.New("invalid project status transition")

type Project struct {
	ID            string
	Name          string
	Description   string
	Status        Status
	BrainID       string
	WorkspacePath string
	RepositoryURL string
}

func New(id, name string) (Project, error) {
	if strings.TrimSpace(id) == "" {
		return Project{}, errors.New("project ID is required")
	}
	if strings.TrimSpace(name) == "" {
		return Project{}, errors.New("project name is required")
	}
	return Project{ID: id, Name: name, Status: StatusCreated}, nil
}

func (p Project) WithBrain(brainID string) (Project, error) {
	if strings.TrimSpace(brainID) == "" {
		return Project{}, errors.New("brain ID is required")
	}
	p.BrainID = brainID
	return p, nil
}

func (p Project) Transition(to Status) (Project, error) {
	if !validStatus(to) {
		return Project{}, fmt.Errorf("unknown project status %q", to)
	}
	if !allowedTransition(p.Status, to) {
		return Project{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, p.Status, to)
	}
	if (to == StatusReady || to == StatusRunning) && p.BrainID == "" {
		return Project{}, errors.New("project requires a brain before becoming ready or running")
	}
	p.Status = to
	return p, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusCreated, StatusConfiguring, StatusReady, StatusRunning, StatusPaused, StatusArchived:
		return true
	default:
		return false
	}
}

func allowedTransition(from, to Status) bool {
	switch from {
	case StatusCreated:
		return to == StatusConfiguring || to == StatusArchived
	case StatusConfiguring:
		return to == StatusReady || to == StatusArchived
	case StatusReady:
		return to == StatusRunning || to == StatusPaused || to == StatusArchived
	case StatusRunning:
		return to == StatusPaused || to == StatusArchived
	case StatusPaused:
		return to == StatusReady || to == StatusRunning || to == StatusArchived
	default:
		return false
	}
}
