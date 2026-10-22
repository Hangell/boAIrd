// Package agent defines provider-independent agent configuration and state.
package agent

import (
	"errors"
	"fmt"
	"strings"

	"boAIrd/internal/capability"
)

type Status string

const (
	StatusOnline  Status = "ONLINE"
	StatusIdle    Status = "IDLE"
	StatusBusy    Status = "BUSY"
	StatusWaiting Status = "WAITING"
	StatusOffline Status = "OFFLINE"
	StatusStale   Status = "STALE"
	StatusError   Status = "ERROR"
)

type Agent struct {
	ID                  string
	Name                string
	Avatar              string
	Role                string
	Description         string
	Runtime             string
	Provider            string
	Model               string
	Instructions        string
	LanguagePreferences []string
	Capabilities        []capability.Capability
	Tools               []string
	Status              Status
	ProjectIDs          []string
}

func New(id, name, role string) (Agent, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(role) == "" {
		return Agent{}, errors.New("agent ID, name, and role are required")
	}
	return Agent{ID: id, Name: name, Role: role, Status: StatusIdle}, nil
}

func (a Agent) SetStatus(status Status) (Agent, error) {
	if !validStatus(status) {
		return Agent{}, fmt.Errorf("unknown agent status %q", status)
	}
	a.Status = status
	return a, nil
}

func (a Agent) AddProject(projectID string) (Agent, error) {
	if strings.TrimSpace(projectID) == "" {
		return Agent{}, errors.New("project ID is required")
	}
	for _, existing := range a.ProjectIDs {
		if existing == projectID {
			return a, nil
		}
	}
	a.ProjectIDs = append(append([]string(nil), a.ProjectIDs...), projectID)
	return a, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusOnline, StatusIdle, StatusBusy, StatusWaiting, StatusOffline, StatusStale, StatusError:
		return true
	default:
		return false
	}
}
