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
	id        string
	projectID string
	agentID   string
	autonomy  AutonomyMode
	active    bool
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
	return Brain{id: id, projectID: projectID, agentID: agentID, autonomy: autonomy, active: true}, nil
}

func (b Brain) ID() string             { return b.id }
func (b Brain) ProjectID() string      { return b.projectID }
func (b Brain) AgentID() string        { return b.agentID }
func (b Brain) Autonomy() AutonomyMode { return b.autonomy }
func (b Brain) Active() bool           { return b.active }

func (b Brain) SetAutonomy(mode AutonomyMode) (Brain, error) {
	if !validAutonomy(mode) {
		return Brain{}, errors.New("invalid autonomy mode")
	}
	b.autonomy = mode
	return b, nil
}

func (b Brain) Deactivate() Brain {
	b.active = false
	return b
}

func (b Brain) Activate() Brain {
	b.active = true
	return b
}

func validAutonomy(mode AutonomyMode) bool {
	switch mode {
	case AutonomyManual, AutonomySemiAuto, AutonomyAutonomous:
		return true
	default:
		return false
	}
}
