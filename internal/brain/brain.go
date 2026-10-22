// Package brain defines provider-independent Brain configuration.
package brain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
	"boAIrd/internal/project"
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

type Action string

const (
	ActionPlan              Action = "plan"
	ActionCreateTask        Action = "task.create"
	ActionAssignTask        Action = "task.assign"
	ActionReviewTask        Action = "task.review"
	ActionCompleteTask      Action = "task.complete"
	ActionTransitionTask    Action = "task.transition"
	ActionRequestAdminHelp  Action = "admin.help.request"
	ActionReplaceBrain      Action = "brain.replace"
	ActionGrantCapability   Action = "capability.grant"
	ActionAccessCredential  Action = "credential.read"
	ActionBypassHumanReview Action = "human_review.bypass"
)

var ErrUnauthorizedAction = errors.New("Brain action is not authorized")
var ErrHumanApprovalRequired = errors.New("Brain action requires human approval")
var (
	ErrNotFound             = errors.New("Brain not found")
	ErrProjectNotFound      = errors.New("project not found")
	ErrProjectUnavailable   = errors.New("project cannot receive a Brain")
	ErrInvalidAgent         = errors.New("agent is not eligible to be a Brain")
	ErrInvalidInput         = errors.New("invalid Brain input")
	ErrAlreadyExists        = errors.New("project already has a Brain")
	ErrConfigurationChanged = errors.New("Brain configuration changed during replacement")
)

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
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

// AuthorizeAction checks identity, project policy, and explicit agent capabilities.
func (b Brain) AuthorizeAction(action Action, p project.Project, a agent.Agent) error {
	if !b.active || b.projectID != p.ID() || p.BrainID() != b.id || b.agentID != a.ID() || p.Status() == project.StatusArchived {
		return ErrUnauthorizedAction
	}
	switch action {
	case ActionReplaceBrain, ActionGrantCapability, ActionAccessCredential, ActionBypassHumanReview:
		return ErrUnauthorizedAction
	}
	var required string
	switch action {
	case ActionPlan:
		required = "brain.execute"
	case ActionCreateTask:
		required = "task.create"
	case ActionAssignTask:
		required = "task.assign"
	case ActionReviewTask:
		required = "task.review"
	case ActionCompleteTask:
		required = "task.complete"
	case ActionTransitionTask:
		required = "task.transition"
	case ActionRequestAdminHelp:
		required = "admin.help.request"
	default:
		return ErrUnauthorizedAction
	}
	value, err := capability.New(required)
	if err != nil {
		return ErrUnauthorizedAction
	}
	if err := (capability.Authorizer{}).Authorize(capability.Principal{
		ID: a.ID(), Capabilities: a.Capabilities(), ProjectIDs: []string{p.ID()},
	}, value, p.ID()); err != nil {
		return ErrUnauthorizedAction
	}
	if err := b.authorizeAutonomy(action, p); err != nil {
		return err
	}
	return nil
}

func (b Brain) authorizeAutonomy(action Action, p project.Project) error {
	switch action {
	case ActionPlan, ActionRequestAdminHelp:
		return nil
	}
	switch b.autonomy {
	case AutonomyManual:
		return ErrHumanApprovalRequired
	case AutonomySemiAuto:
		if action == ActionCompleteTask {
			return ErrHumanApprovalRequired
		}
	case AutonomyAutonomous:
		if action == ActionCompleteTask && p.HumanReviewRequired() {
			return ErrHumanApprovalRequired
		}
	default:
		return ErrUnauthorizedAction
	}
	return nil
}

func validAutonomy(mode AutonomyMode) bool {
	switch mode {
	case AutonomyManual, AutonomySemiAuto, AutonomyAutonomous:
		return true
	default:
		return false
	}
}
