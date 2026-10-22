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
	id           string
	name         string
	role         string
	status       Status
	capabilities []capability.Capability
	projectIDs   []string
	tools        []string

	Avatar              string
	Description         string
	Runtime             string
	Provider            string
	Model               string
	Instructions        string
	LanguagePreferences []string
}

func New(id, name, role string) (Agent, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(role) == "" {
		return Agent{}, errors.New("agent ID, name, and role are required")
	}
	return Agent{id: id, name: name, role: role, status: StatusIdle}, nil
}

func (a Agent) ID() string     { return a.id }
func (a Agent) Name() string   { return a.name }
func (a Agent) Role() string   { return a.role }
func (a Agent) Status() Status { return a.status }

func (a Agent) Capabilities() []capability.Capability {
	return append([]capability.Capability(nil), a.capabilities...)
}

func (a Agent) ProjectIDs() []string {
	return append([]string(nil), a.projectIDs...)
}

func (a Agent) Tools() []string {
	return append([]string(nil), a.tools...)
}

func (a Agent) SetStatus(status Status) (Agent, error) {
	if !validStatus(status) {
		return Agent{}, fmt.Errorf("unknown agent status %q", status)
	}
	a.status = status
	return a, nil
}

func (a Agent) Rename(name string) (Agent, error) {
	if strings.TrimSpace(name) == "" {
		return Agent{}, errors.New("agent name is required")
	}
	a.name = name
	return a, nil
}

func (a Agent) SetRole(role string) (Agent, error) {
	if strings.TrimSpace(role) == "" {
		return Agent{}, errors.New("agent role is required")
	}
	a.role = role
	return a, nil
}

func (a Agent) AddProject(projectID string) (Agent, error) {
	if strings.TrimSpace(projectID) == "" {
		return Agent{}, errors.New("project ID is required")
	}
	projectID = strings.TrimSpace(projectID)
	for _, existing := range a.projectIDs {
		if existing == projectID {
			return a, nil
		}
	}
	a.projectIDs = append(append([]string(nil), a.projectIDs...), projectID)
	return a, nil
}

func (a Agent) AddCapability(value capability.Capability) (Agent, error) {
	if !value.Valid() {
		return Agent{}, errors.New("invalid capability")
	}
	for _, existing := range a.capabilities {
		if existing == value {
			return a, nil
		}
	}
	a.capabilities = append(append([]capability.Capability(nil), a.capabilities...), value)
	return a, nil
}

func (a Agent) AddTool(name string) (Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Agent{}, errors.New("tool name is required")
	}
	for _, existing := range a.tools {
		if existing == name {
			return a, nil
		}
	}
	a.tools = append(append([]string(nil), a.tools...), name)
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
