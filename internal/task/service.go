package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/capability"
	"boAIrd/internal/project"
)

var (
	ErrNotFound        = errors.New("task not found")
	ErrProjectNotFound = errors.New("project not found")
	ErrForbidden       = errors.New("task action is not authorized")
	ErrInvalidInput    = errors.New("invalid task input")
	ErrProjectArchived = errors.New("project is archived")
)

type Repository interface {
	Create(context.Context, Task) error
	Get(context.Context, string) (Task, error)
	List(context.Context, string, Status, string, int, int) ([]Task, error)
	Update(context.Context, Task) error
	ListAuditEvents(context.Context, string) ([]AuditEvent, error)
}
type ProjectReader interface {
	Get(context.Context, string) (project.Project, error)
}
type AgentReader interface {
	Get(context.Context, string) (agent.Agent, error)
}
type BrainReader interface {
	GetActiveByProject(context.Context, string) (brain.Brain, error)
}
type authorizationDenialRecorder interface {
	RecordAuthorizationDenial(context.Context, string, string, string, string, string) error
}

type Service struct {
	tasks    Repository
	projects ProjectReader
	agents   AgentReader
	brains   BrainReader
}

type CreateInput struct {
	Title       string
	Description string
	Priority    string
	ParentID    string
	Reason      string
}

type AuditEvent struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	TaskID     string    `json:"task_id,omitempty"`
	ActorType  ActorType `json:"actor_type"`
	ActorID    string    `json:"actor_id"`
	EventType  string    `json:"event_type"`
	FromStatus string    `json:"from_status,omitempty"`
	ToStatus   string    `json:"to_status,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func NewService(tasks Repository, projects ProjectReader, agents AgentReader, brains BrainReader) (*Service, error) {
	if tasks == nil || projects == nil || agents == nil || brains == nil {
		return nil, errors.New("task service dependencies are required")
	}
	return &Service{tasks: tasks, projects: projects, agents: agents, brains: brains}, nil
}

func (s *Service) Create(ctx context.Context, projectID string, input CreateInput, actor Actor) (Task, error) {
	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return Task{}, mapProjectError(err)
	}
	if p.Status() == project.StatusArchived {
		return Task{}, ErrProjectArchived
	}
	if err := s.authorizeCapability(ctx, projectID, actor, "task.create"); err != nil {
		s.recordAuthorizationDenial(ctx, actor, projectID, "task.create", "project:"+projectID, "capability_denied")
		return Task{}, err
	}
	if strings.TrimSpace(input.Reason) == "" {
		return Task{}, invalidTaskInput(errors.New("reason is required"))
	}
	if strings.TrimSpace(input.ParentID) != "" {
		parent, err := s.tasks.Get(ctx, strings.TrimSpace(input.ParentID))
		if err != nil {
			return Task{}, err
		}
		if parent.ProjectID() != projectID {
			return Task{}, invalidTaskInput(errors.New("parent task must belong to the same project"))
		}
	}
	id, err := newTaskID()
	if err != nil {
		return Task{}, fmt.Errorf("generate task ID: %w", err)
	}
	value, err := NewWithActor(id, projectID, input.Title, input.Description, actor)
	if err != nil {
		return Task{}, invalidTaskInput(err)
	}
	value, err = value.SetPriority(input.Priority)
	if err != nil {
		return Task{}, invalidTaskInput(err)
	}
	if p.HumanReviewRequired() {
		value, err = value.RequireHumanReview()
		if err != nil {
			return Task{}, err
		}
	}
	if input.ParentID != "" {
		value, err = value.SetParent(input.ParentID)
		if err != nil {
			return Task{}, invalidTaskInput(err)
		}
	}
	value, err = value.RecordMutation(actor, input.Reason)
	if err != nil {
		return Task{}, invalidTaskInput(err)
	}
	if err := s.tasks.Create(ctx, value); err != nil {
		return Task{}, err
	}
	return value, nil
}

func (s *Service) Get(ctx context.Context, id string) (Task, error) {
	return s.tasks.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) List(ctx context.Context, projectID string, status Status, assignedTo string, limit, offset int) ([]Task, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		return nil, mapProjectError(err)
	}
	if status != "" && !validStatus(status) {
		return nil, invalidTaskInput(errors.New("invalid task status"))
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, invalidTaskInput(errors.New("limit must be 1 to 100 and offset non-negative"))
	}
	return s.tasks.List(ctx, projectID, status, strings.TrimSpace(assignedTo), limit, offset)
}

func (s *Service) Assign(ctx context.Context, id, agentID, reason string, actor Actor) (Task, error) {
	value, err := s.tasks.Get(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if err := s.ensureProjectActive(ctx, value.ProjectID()); err != nil {
		return Task{}, err
	}
	if err := s.authorizeCapability(ctx, value.ProjectID(), actor, "task.assign"); err != nil {
		s.recordAuthorizationDenial(ctx, actor, value.ProjectID(), "task.assign", "task:"+value.ID(), "capability_denied")
		return Task{}, err
	}
	if strings.TrimSpace(agentID) == "" {
		return Task{}, invalidTaskInput(errors.New("agent ID is required"))
	}
	if _, err := s.agents.Get(ctx, agentID); err != nil {
		return Task{}, err
	}
	value, err = value.Assign(agentID)
	if err != nil {
		return Task{}, invalidTaskInput(err)
	}
	value, err = value.RecordMutation(actor, reason)
	if err != nil {
		return Task{}, invalidTaskInput(err)
	}
	if err := s.tasks.Update(ctx, value); err != nil {
		return Task{}, err
	}
	return value, nil
}

func (s *Service) Transition(ctx context.Context, id string, to Status, reason string, actor Actor) (Task, error) {
	value, err := s.tasks.Get(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if err := s.ensureProjectActive(ctx, value.ProjectID()); err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return Task{}, invalidTaskInput(errors.New("reason is required"))
	}
	if to == StatusHumanReview && !value.HumanReview() {
		s.recordAuthorizationDenial(ctx, actor, value.ProjectID(), "task.review", "task:"+value.ID(), "human_review_policy_denied")
		return Task{}, ErrForbidden
	}
	if value.HumanReview() && to == StatusDone && actor.Type != ActorLocalAPI && actor.Type != ActorAdministrator {
		s.recordAuthorizationDenial(ctx, actor, value.ProjectID(), "task.complete", "task:"+value.ID(), "human_review_required")
		return Task{}, ErrForbidden
	}
	if err := s.authorizeTransition(ctx, value, to, actor); err != nil {
		s.recordAuthorizationDenial(ctx, actor, value.ProjectID(), transitionCapability(to), "task:"+value.ID(), "capability_denied")
		return Task{}, err
	}
	value, err = value.Transition(to)
	if err != nil {
		return Task{}, err
	}
	value, err = value.RecordMutation(actor, reason)
	if err != nil {
		return Task{}, invalidTaskInput(err)
	}
	if err := s.tasks.Update(ctx, value); err != nil {
		return Task{}, err
	}
	return value, nil
}

func (s *Service) recordAuthorizationDenial(ctx context.Context, actor Actor, projectID, required, resource, reason string) {
	recorder, ok := s.tasks.(authorizationDenialRecorder)
	if !ok {
		return
	}
	if err := recorder.RecordAuthorizationDenial(ctx, actor.ID, projectID, required, resource, reason); err != nil {
		slog.ErrorContext(ctx, "record task authorization denial", "error", err)
	}
}

func transitionCapability(to Status) string {
	switch to {
	case StatusReview, StatusHumanReview, StatusBlocked, StatusFailed, StatusCancelled:
		return "task.review"
	case StatusDone:
		return "task.complete"
	default:
		return "task.transition"
	}
}

func (s *Service) ListAuditEvents(ctx context.Context, id string) ([]AuditEvent, error) {
	if _, err := s.tasks.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.tasks.ListAuditEvents(ctx, id)
}

func (s *Service) ensureProjectActive(ctx context.Context, projectID string) error {
	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return mapProjectError(err)
	}
	if p.Status() == project.StatusArchived {
		return ErrProjectArchived
	}
	return nil
}

func (s *Service) authorizeCapability(ctx context.Context, projectID string, actor Actor, required string) error {
	switch actor.Type {
	case ActorAdministrator:
		value, err := capability.New(required)
		if err != nil {
			return ErrForbidden
		}
		grants := make([]capability.Capability, 0, len(actor.Capabilities))
		for _, raw := range actor.Capabilities {
			grant, err := capability.New(raw)
			if err == nil {
				grants = append(grants, grant)
			}
		}
		if err := (capability.Authorizer{}).Authorize(capability.Principal{ID: actor.ID, Capabilities: grants, ProjectIDs: actor.ProjectIDs}, value, projectID); err != nil {
			return ErrForbidden
		}
		return nil
	case ActorLocalAPI:
		return ErrForbidden
	case ActorBrain:
		configured, err := s.brains.GetActiveByProject(ctx, projectID)
		if err != nil || configured.AgentID() != actor.ID {
			return ErrForbidden
		}
		return s.agentHasCapability(ctx, projectID, actor.ID, required, true)
	case ActorAgent:
		return s.agentHasCapability(ctx, projectID, actor.ID, required, false)
	}
	return ErrForbidden
}

func (s *Service) authorizeTransition(ctx context.Context, value Task, to Status, actor Actor) error {
	if actor.Type == ActorLocalAPI || actor.Type == ActorAdministrator {
		required := "task.transition"
		switch to {
		case StatusReview, StatusHumanReview, StatusBlocked, StatusFailed, StatusCancelled:
			required = "task.review"
		case StatusDone:
			required = "task.complete"
		}
		return s.authorizeCapability(ctx, value.ProjectID(), actor, required)
	}
	if actor.Type == ActorAgent {
		if actor.ID != value.AssignedTo() {
			return ErrForbidden
		}
		if err := s.agentHasAssignedTaskCapability(ctx, value, actor.ID, "task.transition"); err != nil {
			return ErrForbidden
		}
		switch to {
		case StatusWorking, StatusReview, StatusBlocked, StatusFailed, StatusCancelled:
			return s.agentExists(ctx, actor.ID)
		default:
			return ErrForbidden
		}
	}
	if actor.Type != ActorBrain {
		return ErrForbidden
	}
	required := "task.assign"
	switch to {
	case StatusReview, StatusHumanReview, StatusDone, StatusBlocked, StatusFailed, StatusCancelled:
		required = "task.review"
	}
	if to == StatusDone && !value.HumanReview() {
		required = "task.complete"
	}
	return s.authorizeCapability(ctx, value.ProjectID(), actor, required)
}

func (s *Service) agentExists(ctx context.Context, id string) error {
	if _, err := s.agents.Get(ctx, id); err != nil {
		return ErrForbidden
	}
	return nil
}

func (s *Service) agentHasCapability(ctx context.Context, projectID, id, required string, activeBrain bool) error {
	a, err := s.agents.Get(ctx, id)
	if err != nil {
		return ErrForbidden
	}
	scope := a.ProjectIDs()
	if activeBrain {
		scope = []string{projectID}
	}
	return authorizeAgentCapability(a, projectID, required, scope)
}

func (s *Service) agentHasAssignedTaskCapability(ctx context.Context, value Task, id, required string) error {
	if value.AssignedTo() != id {
		return ErrForbidden
	}
	a, err := s.agents.Get(ctx, id)
	if err != nil {
		return ErrForbidden
	}
	scope := append(a.ProjectIDs(), value.ProjectID())
	return authorizeAgentCapability(a, value.ProjectID(), required, scope)
}

func authorizeAgentCapability(a agent.Agent, projectID, required string, scope []string) error {
	grant, err := capability.New(required)
	if err != nil {
		return ErrForbidden
	}
	grants := make([]capability.Capability, 0, len(a.Capabilities()))
	for _, value := range a.Capabilities() {
		grants = append(grants, value)
	}
	if err := (capability.Authorizer{}).Authorize(capability.Principal{ID: a.ID(), Capabilities: grants, ProjectIDs: scope}, grant, projectID); err != nil {
		return ErrForbidden
	}
	return nil
}

func mapProjectError(err error) error {
	if errors.Is(err, project.ErrNotFound) {
		return ErrProjectNotFound
	}
	return err
}

func invalidTaskInput(err error) error { return fmt.Errorf("%w: %w", ErrInvalidInput, err) }

func newTaskID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
