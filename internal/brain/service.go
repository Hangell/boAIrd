package brain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"boAIrd/internal/agent"
	"boAIrd/internal/project"
	"boAIrd/internal/provider"
	"boAIrd/internal/provider/adapters"
	"boAIrd/internal/run"
)

type Repository interface {
	Create(context.Context, Brain) error
	GetActiveByProject(context.Context, string) (Brain, error)
}
type ProjectReader interface {
	Get(context.Context, string) (project.Project, error)
}
type AgentReader interface {
	Get(context.Context, string) (agent.Agent, error)
}
type RunRepository interface {
	Create(context.Context, run.Run) error
	Update(context.Context, run.Run) error
}
type CredentialResolver interface {
	Resolve(context.Context, string) (string, error)
}

type Service struct {
	brains      Repository
	projects    ProjectReader
	agents      AgentReader
	runs        RunRepository
	credentials CredentialResolver
}

type CreateInput struct {
	AgentID  string
	Autonomy AutonomyMode
}
type Execution struct {
	Run     run.Run
	Content string
}

func NewService(brains Repository, projects ProjectReader, agents AgentReader, runs RunRepository, credentials CredentialResolver) (*Service, error) {
	if brains == nil || projects == nil || agents == nil || runs == nil || credentials == nil {
		return nil, errors.New("Brain service dependencies are required")
	}
	return &Service{brains: brains, projects: projects, agents: agents, runs: runs, credentials: credentials}, nil
}

func (s *Service) Create(ctx context.Context, projectID string, input CreateInput) (Brain, error) {
	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return Brain{}, err
	}
	if p.Status() == project.StatusArchived {
		return Brain{}, ErrProjectUnavailable
	}
	a, err := s.agents.Get(ctx, input.AgentID)
	if err != nil {
		return Brain{}, err
	}
	if a.Runtime() != agent.RuntimeAPI {
		return Brain{}, ErrInvalidAgent
	}
	allowed := false
	for _, capability := range a.Capabilities() {
		if string(capability) == "brain.execute" {
			allowed = true
			break
		}
	}
	if !allowed {
		return Brain{}, ErrInvalidAgent
	}
	id, err := newID()
	if err != nil {
		return Brain{}, fmt.Errorf("generate Brain ID: %w", err)
	}
	value, err := New(id, projectID, a.ID(), input.Autonomy)
	if err != nil {
		return Brain{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := s.brains.Create(ctx, value); err != nil {
		return Brain{}, err
	}
	return value, nil
}

func (s *Service) Get(ctx context.Context, projectID string) (Brain, error) {
	return s.brains.GetActiveByProject(ctx, projectID)
}

func (s *Service) Execute(ctx context.Context, projectID, objective string, maxTokens int) (Execution, error) {
	objective = strings.TrimSpace(objective)
	if objective == "" || len(objective) > 16<<10 {
		return Execution{}, fmt.Errorf("%w: objective must contain 1 to 16384 bytes", ErrInvalidInput)
	}
	if maxTokens == 0 {
		maxTokens = 2048
	}
	if maxTokens < 1 || maxTokens > 4096 {
		return Execution{}, fmt.Errorf("%w: max_tokens must be between 1 and 4096", ErrInvalidInput)
	}
	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		return Execution{}, err
	}
	b, err := s.brains.GetActiveByProject(ctx, projectID)
	if err != nil {
		return Execution{}, err
	}
	a, err := s.agents.Get(ctx, b.AgentID())
	if err != nil {
		return Execution{}, err
	}
	if err := b.AuthorizeAction(ActionPlan, p, a); err != nil {
		return Execution{}, err
	}
	registry := provider.NewRegistry()
	if err := adapters.Register(registry); err != nil {
		return Execution{}, errors.New("initialize provider registry")
	}
	if err := registry.Configure(a.ID(), a.Provider(), provider.Config{BaseURL: a.ProviderBaseURL(), CredentialID: a.CredentialID(), CredentialResolver: s.credentials}); err != nil {
		return Execution{}, errors.New("configure Brain provider")
	}
	client, err := registry.Resolve(a.ID())
	if err != nil {
		return Execution{}, errors.New("resolve Brain provider")
	}
	id, err := newID()
	if err != nil {
		return Execution{}, errors.New("generate run ID")
	}
	input := "Objective: " + objective
	value, err := run.NewBrainRun(id, projectID, a.ID())
	if err != nil {
		return Execution{}, err
	}
	value, err = value.ConfigureExecution(string(a.Runtime()), a.Provider(), a.Model(), input)
	if err != nil {
		return Execution{}, err
	}
	if err := s.runs.Create(ctx, value); err != nil {
		return Execution{}, fmt.Errorf("persist pending run: %w", err)
	}
	value, err = value.Transition(run.StatusRunning)
	if err != nil {
		return Execution{}, err
	}
	if err := s.runs.Update(ctx, value); err != nil {
		return Execution{}, fmt.Errorf("persist running run: %w", err)
	}
	system := "You are boAIrd's project Brain. Return a proposed plan as text only. Do not claim to perform actions. You cannot change project policy, permissions, capabilities, credentials, Brain assignment, or human review requirements. All task mutations require separately authorized tools and human review where configured."
	if instructions := strings.TrimSpace(a.Instructions()); instructions != "" {
		system += "\nAgent instructions:\n" + instructions
	}
	response, callErr := client.Chat(ctx, provider.ChatRequest{Model: a.Model(), MaxTokens: maxTokens, Messages: []provider.ChatMessage{{Role: provider.RoleSystem, Content: system}, {Role: provider.RoleUser, Content: objective}}})
	if callErr != nil {
		value, _ = value.SetError("provider request failed")
		value, _ = value.Transition(run.StatusFailed)
		if persistErr := s.runs.Update(context.WithoutCancel(ctx), value); persistErr != nil {
			return Execution{}, fmt.Errorf("provider request failed; persist failed run: %w", persistErr)
		}
		return Execution{Run: value}, errors.New("Brain provider request failed")
	}
	value, err = value.SetOutput(response.Content)
	if err == nil {
		value, err = value.Transition(run.StatusSuccess)
	}
	if err != nil {
		return Execution{}, err
	}
	if err := s.runs.Update(ctx, value); err != nil {
		return Execution{}, fmt.Errorf("persist successful run: %w", err)
	}
	return Execution{Run: value, Content: response.Content}, nil
}
