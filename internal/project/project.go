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
	id            string
	name          string
	description   string
	status        Status
	brainID       string
	workspacePath string
	repositoryURL string
}

func New(id, name string) (Project, error) {
	if strings.TrimSpace(id) == "" {
		return Project{}, errors.New("project ID is required")
	}
	if strings.TrimSpace(name) == "" {
		return Project{}, errors.New("project name is required")
	}
	return Project{id: id, name: name, status: StatusCreated}, nil
}

func (p Project) ID() string            { return p.id }
func (p Project) Name() string          { return p.name }
func (p Project) Description() string   { return p.description }
func (p Project) Status() Status        { return p.status }
func (p Project) BrainID() string       { return p.brainID }
func (p Project) WorkspacePath() string { return p.workspacePath }
func (p Project) RepositoryURL() string { return p.repositoryURL }

func (p Project) Rename(name string) (Project, error) {
	if strings.TrimSpace(name) == "" {
		return Project{}, errors.New("project name is required")
	}
	p.name = name
	return p, nil
}

func (p Project) SetDescription(description string) Project {
	p.description = description
	return p
}

func (p Project) SetWorkspacePath(path string) Project {
	p.workspacePath = strings.TrimSpace(path)
	return p
}

func (p Project) SetRepositoryURL(url string) Project {
	p.repositoryURL = strings.TrimSpace(url)
	return p
}

func (p Project) WithBrain(brainID string) (Project, error) {
	if strings.TrimSpace(brainID) == "" {
		return Project{}, errors.New("brain ID is required")
	}
	p.brainID = strings.TrimSpace(brainID)
	return p, nil
}

func (p Project) WithoutBrain() (Project, error) {
	if p.status == StatusReady || p.status == StatusRunning {
		return Project{}, errors.New("cannot remove Brain while project is ready or running")
	}
	p.brainID = ""
	return p, nil
}

func (p Project) Transition(to Status) (Project, error) {
	if !validStatus(to) {
		return Project{}, fmt.Errorf("unknown project status %q", to)
	}
	if !allowedTransition(p.status, to) {
		return Project{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, p.status, to)
	}
	if (to == StatusReady || to == StatusRunning) && p.brainID == "" {
		return Project{}, errors.New("project requires a brain before becoming ready or running")
	}
	p.status = to
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
