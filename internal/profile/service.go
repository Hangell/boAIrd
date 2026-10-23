package profile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/project"
)

const (
	maxInstructionsBytes = 16000
	maxCapabilities      = 100
	maxTools             = 100
)

// Override is a project-scoped customization of a built-in profile. Its
// capabilities and tools are the only ones granted; they come from explicit
// administrator configuration, never from the profile name.
type Override struct {
	ID           string
	ProjectID    string
	ProfileKey   string
	Instructions string
	Capabilities []string
	Tools        []string
	CreatedBy    string
	UpdatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Effective resolves a built-in definition with a project override.
type Effective struct {
	Key                   string
	Name                  string
	Description           string
	Instructions          string
	DefaultRuntime        agent.Runtime
	Capabilities          []string
	Tools                 []string
	SuggestedCapabilities []string
	SuggestedTools        []string
	Customized            bool
}

// SaveInput carries the administrator-configured project override.
type SaveInput struct {
	Instructions string
	Capabilities []string
	Tools        []string
	ActorID      string
}

// AuditEvent describes a profile configuration change for immutable audit.
type AuditEvent struct {
	ProjectID  string
	ProfileKey string
	ActorID    string
	Action     string
}

type Repository interface {
	Get(context.Context, string, string) (Override, error)
	Save(context.Context, Override) error
	List(context.Context, string) ([]Override, error)
}

type ProjectReader interface {
	Get(context.Context, string) (project.Project, error)
}

type AuditRecorder interface {
	RecordProfileChange(context.Context, AuditEvent) error
}

type Service struct {
	repository Repository
	projects   ProjectReader
	audit      AuditRecorder
}

func NewService(repository Repository, projects ProjectReader, audit AuditRecorder) (*Service, error) {
	if repository == nil {
		return nil, errors.New("profile repository is required")
	}
	if projects == nil {
		return nil, errors.New("project reader is required")
	}
	if audit == nil {
		return nil, errors.New("profile audit recorder is required")
	}
	return &Service{repository: repository, projects: projects, audit: audit}, nil
}

// Effective resolves a profile for a project. With no override, capabilities
// and tools are empty: a profile name grants nothing.
func (s *Service) Effective(ctx context.Context, projectID, key string) (Effective, error) {
	if _, err := s.ensureProjectActive(ctx, projectID); err != nil {
		return Effective{}, err
	}
	definition, ok := Lookup(key)
	if !ok {
		return Effective{}, fmt.Errorf("%w: %q", ErrUnknownProfile, key)
	}
	effective := fromDefinition(definition)
	override, err := s.repository.Get(ctx, projectID, definition.Key)
	if errors.Is(err, ErrNotFound) {
		return effective, nil
	}
	if err != nil {
		return Effective{}, err
	}
	return applyOverride(effective, override), nil
}

// ListEffective returns every built-in profile resolved for one project.
func (s *Service) ListEffective(ctx context.Context, projectID string) ([]Effective, error) {
	if _, err := s.ensureProjectActive(ctx, projectID); err != nil {
		return nil, err
	}
	overrides, err := s.repository.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]Override, len(overrides))
	for _, override := range overrides {
		byKey[override.ProfileKey] = override
	}
	values := make([]Effective, 0, len(Builtins()))
	for _, definition := range Builtins() {
		effective := fromDefinition(definition)
		if override, ok := byKey[definition.Key]; ok {
			effective = applyOverride(effective, override)
		}
		values = append(values, effective)
	}
	return values, nil
}

// Save validates and persists an administrator-configured project override and
// records the change in the immutable audit history.
func (s *Service) Save(ctx context.Context, projectID, key string, input SaveInput) (Effective, error) {
	if _, err := s.ensureProjectActive(ctx, projectID); err != nil {
		return Effective{}, err
	}
	definition, ok := Lookup(key)
	if !ok {
		return Effective{}, fmt.Errorf("%w: %q", ErrUnknownProfile, key)
	}
	input.ActorID = strings.TrimSpace(input.ActorID)
	if input.ActorID == "" {
		return Effective{}, fmt.Errorf("%w: administrator actor is required", ErrInvalidInput)
	}
	instructions := strings.TrimSpace(input.Instructions)
	if len(instructions) > maxInstructionsBytes {
		return Effective{}, fmt.Errorf("%w: instructions must not exceed %d bytes", ErrInvalidInput, maxInstructionsBytes)
	}
	if len(input.Capabilities) > maxCapabilities {
		return Effective{}, fmt.Errorf("%w: capabilities must contain at most %d values", ErrInvalidInput, maxCapabilities)
	}
	if len(input.Tools) > maxTools {
		return Effective{}, fmt.Errorf("%w: tools must contain at most %d values", ErrInvalidInput, maxTools)
	}
	capabilities, err := ValidCapabilities(input.Capabilities)
	if err != nil {
		return Effective{}, err
	}
	tools, err := definition.ValidateTools(input.Tools)
	if err != nil {
		return Effective{}, err
	}

	now := time.Now().UTC()
	action := "created"
	createdAt := now
	existing, err := s.repository.Get(ctx, projectID, definition.Key)
	switch {
	case err == nil:
		action = "updated"
		createdAt = existing.CreatedAt
	case errors.Is(err, ErrNotFound):
		// new override
	default:
		return Effective{}, err
	}

	id, err := newID()
	if err != nil {
		return Effective{}, fmt.Errorf("generate profile override ID: %w", err)
	}
	if existing.ID != "" {
		id = existing.ID
	}
	override := Override{
		ID: id, ProjectID: projectID, ProfileKey: definition.Key, Instructions: instructions,
		Capabilities: capabilities, Tools: tools, CreatedBy: existing.CreatedBy, UpdatedBy: input.ActorID,
		CreatedAt: createdAt, UpdatedAt: now,
	}
	if action == "created" {
		override.CreatedBy = input.ActorID
	}
	if err := s.repository.Save(ctx, override); err != nil {
		return Effective{}, err
	}
	if err := s.audit.RecordProfileChange(ctx, AuditEvent{
		ProjectID: projectID, ProfileKey: definition.Key, ActorID: input.ActorID, Action: action,
	}); err != nil {
		return Effective{}, err
	}
	return applyOverride(fromDefinition(definition), override), nil
}

func (s *Service) ensureProjectActive(ctx context.Context, projectID string) (project.Project, error) {
	value, err := s.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return project.Project{}, ErrProjectNotFound
		}
		return project.Project{}, err
	}
	if value.Status() == project.StatusArchived {
		return project.Project{}, ErrProjectArchived
	}
	return value, nil
}

func fromDefinition(definition Definition) Effective {
	return Effective{
		Key: definition.Key, Name: definition.Name, Description: definition.Description,
		Instructions: definition.Instructions, DefaultRuntime: definition.DefaultRuntime,
		SuggestedCapabilities: append([]string(nil), definition.SuggestedCapabilities...),
		SuggestedTools:        append([]string(nil), definition.SuggestedTools...),
	}
}

func applyOverride(effective Effective, override Override) Effective {
	if override.Instructions != "" {
		effective.Instructions = override.Instructions
	}
	effective.Capabilities = append([]string(nil), override.Capabilities...)
	effective.Tools = append([]string(nil), override.Tools...)
	effective.Customized = true
	return effective
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
