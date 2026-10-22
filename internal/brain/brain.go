// Package brain defines provider-independent Brain configuration.
package brain

import (
	"errors"
	"strings"
)

type AutonomyMode string

const (
	AutonomyManual     AutonomyMode = "MANUAL"
	AutonomySemiAuto   AutonomyMode = "SEMI_AUTO"
	AutonomyAutonomous AutonomyMode = "AUTONOMOUS"
)

type Brain struct {
	ID        string
	ProjectID string
	AgentID   string
	Autonomy  AutonomyMode
	Active    bool
}

func New(id, projectID, agentID string, autonomy AutonomyMode) (Brain, error) {
	if strings.TrimSpace(id) == "" {
		return Brain{}, errors.New("brain ID is required")
	}
	if strings.TrimSpace(projectID) == "" {
		return Brain{}, errors.New("project ID is required")
	}
	if strings.TrimSpace(agentID) == "" {
		return Brain{}, errors.New("agent ID is required")
	}
	if !validAutonomy(autonomy) {
		return Brain{}, errors.New("invalid autonomy mode")
	}
	return Brain{ID: id, ProjectID: projectID, AgentID: agentID, Autonomy: autonomy, Active: true}, nil
}

func (b Brain) SetAutonomy(mode AutonomyMode) (Brain, error) {
	if !validAutonomy(mode) {
		return Brain{}, errors.New("invalid autonomy mode")
	}
	b.Autonomy = mode
	return b, nil
}

func validAutonomy(mode AutonomyMode) bool {
	switch mode {
	case AutonomyManual, AutonomySemiAuto, AutonomyAutonomous:
		return true
	default:
		return false
	}
}
