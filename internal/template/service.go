package template

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/agent"
)

const (
	maxNameBytes         = 200
	maxRoleBytes         = 100
	maxDescriptionBytes  = 4000
	maxInstructionsBytes = 16000
	maxSuggestedValues   = 100
)

type Repository interface {
	CreateWithAudit(context.Context, Template, AuditEvent) error
	UpdateWithAudit(context.Context, Template, int, AuditEvent) error
	Get(context.Context, string) (Template, error)
	List(context.Context) ([]Template, error)
	InstantiateAgent(context.Context, agent.Agent, Instance, AuditEvent) (agent.Agent, error)
}

type Service struct {
	repository Repository
}

type CreateInput struct {
	Key                   string        `json:"key"`
	Name                  string        `json:"name"`
	Role                  string        `json:"role"`
	Description           string        `json:"description"`
	Instructions          string        `json:"instructions"`
	DefaultRuntime        agent.Runtime `json:"default_runtime"`
	SuggestedTools        []string      `json:"suggested_tools"`
	SuggestedCapabilities []string      `json:"suggested_capabilities"`
}

type UpdateInput struct {
	Name                  string        `json:"name"`
	Role                  string        `json:"role"`
	Description           string        `json:"description"`
	Instructions          string        `json:"instructions"`
	DefaultRuntime        agent.Runtime `json:"default_runtime"`
	SuggestedTools        []string      `json:"suggested_tools"`
	SuggestedCapabilities []string      `json:"suggested_capabilities"`
}

type InstantiateInput struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Instructions    string   `json:"instructions"`
	Capabilities    []string `json:"capabilities"`
	Provider        string   `json:"provider,omitempty"`
	ProviderBaseURL string   `json:"provider_base_url,omitempty"`
	Model           string   `json:"model,omitempty"`
	CredentialID    string   `json:"credential_id,omitempty"`
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("template repository is required")
	}
	return &Service{repository: repository}, nil
}

// Create persists a new versioned template and audits the change.
func (s *Service) Create(ctx context.Context, input CreateInput, actorID string) (Template, error) {
	if actorID = strings.TrimSpace(actorID); actorID == "" {
		return Template{}, fmt.Errorf("%w: administrator actor is required", ErrInvalidInput)
	}
	if err := validateKey(input.Key); err != nil {
		return Template{}, err
	}
	value, err := buildTemplate(input, 1, actorID)
	if err != nil {
		return Template{}, err
	}
	if _, err := s.repository.Get(ctx, value.Key); err == nil {
		return Template{}, fmt.Errorf("%w: template %q already exists", ErrInvalidInput, value.Key)
	} else if !errors.Is(err, ErrNotFound) {
		return Template{}, err
	}
	id, err := newID()
	if err != nil {
		return Template{}, fmt.Errorf("generate template ID: %w", err)
	}
	value.ID = id
	now := time.Now().UTC()
	value.CreatedAt, value.UpdatedAt = now, now
	value.CreatedBy, value.UpdatedBy = actorID, actorID
	if err := s.repository.CreateWithAudit(ctx, value, AuditEvent{ActorID: actorID, Action: "created", TemplateKey: value.Key, Version: value.Version}); err != nil {
		return Template{}, err
	}
	return value, nil
}

// Get returns one template by key.
func (s *Service) Get(ctx context.Context, key string) (Template, error) {
	if err := validateKey(key); err != nil {
		return Template{}, err
	}
	return s.repository.Get(ctx, strings.TrimSpace(key))
}

// List returns every template ordered by key.
func (s *Service) List(ctx context.Context) ([]Template, error) {
	return s.repository.List(ctx)
}

// Update bumps the template version and audits the change.
func (s *Service) Update(ctx context.Context, key string, input UpdateInput, actorID string) (Template, error) {
	if actorID = strings.TrimSpace(actorID); actorID == "" {
		return Template{}, fmt.Errorf("%w: administrator actor is required", ErrInvalidInput)
	}
	if err := validateKey(key); err != nil {
		return Template{}, err
	}
	existing, err := s.repository.Get(ctx, strings.TrimSpace(key))
	if err != nil {
		return Template{}, err
	}
	next := Template{
		ID: existing.ID, Key: existing.Key, Name: input.Name, Role: input.Role,
		Description: input.Description, Instructions: input.Instructions,
		DefaultRuntime:        input.DefaultRuntime,
		SuggestedTools:        append([]string(nil), input.SuggestedTools...),
		SuggestedCapabilities: append([]string(nil), input.SuggestedCapabilities...),
		Version:               existing.Version + 1, CreatedBy: existing.CreatedBy, CreatedAt: existing.CreatedAt,
	}
	value, err := buildTemplateFromParts(next)
	if err != nil {
		return Template{}, err
	}
	now := time.Now().UTC()
	value.UpdatedAt = now
	value.UpdatedBy = actorID
	if err := s.repository.UpdateWithAudit(ctx, value, existing.Version, AuditEvent{ActorID: actorID, Action: "updated", TemplateKey: value.Key, Version: value.Version}); err != nil {
		return Template{}, err
	}
	return value, nil
}

// Instantiate creates a separate project Agent from a template. Capabilities
// are granted only from the explicit input; the source template is unchanged.
// Agent, project scope, template provenance, and audit are written atomically.
func (s *Service) Instantiate(ctx context.Context, projectID, key string, input InstantiateInput, actorID string) (agent.Agent, error) {
	if actorID = strings.TrimSpace(actorID); actorID == "" {
		return agent.Agent{}, fmt.Errorf("%w: administrator actor is required", ErrInvalidInput)
	}
	value, err := s.repository.Get(ctx, strings.TrimSpace(key))
	if err != nil {
		return agent.Agent{}, err
	}
	capabilities, err := ValidateCapabilities(input.Capabilities)
	if err != nil {
		return agent.Agent{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = value.Name
	}
	instructions := strings.TrimSpace(input.Instructions)
	if instructions == "" {
		instructions = value.Instructions
	}
	id, err := newID()
	if err != nil {
		return agent.Agent{}, fmt.Errorf("generate agent ID: %w", err)
	}
	configured, err := agent.NewConfigured(id, agent.Configuration{
		Name: name, Role: value.Role, Runtime: value.DefaultRuntime,
		Description: input.Description, Instructions: instructions,
		Provider: input.Provider, ProviderBaseURL: input.ProviderBaseURL,
		Model: input.Model, CredentialID: input.CredentialID,
		Capabilities: capabilities,
	})
	if err != nil {
		return agent.Agent{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	instance := Instance{
		AgentID: id, ProjectID: projectID, TemplateKey: value.Key,
		TemplateVersion: value.Version, CreatedAt: time.Now().UTC(),
	}
	return s.repository.InstantiateAgent(ctx, configured, instance, AuditEvent{
		ActorID: actorID, Action: "instantiated", TemplateKey: value.Key, ProjectID: projectID, Version: value.Version,
	})
}

func buildTemplate(input CreateInput, version int, actorID string) (Template, error) {
	return buildTemplateFromParts(Template{
		Key: input.Key, Name: input.Name, Role: input.Role, Description: input.Description,
		Instructions: input.Instructions, DefaultRuntime: input.DefaultRuntime,
		SuggestedTools:        append([]string(nil), input.SuggestedTools...),
		SuggestedCapabilities: append([]string(nil), input.SuggestedCapabilities...),
		Version:               version, CreatedBy: actorID, UpdatedBy: actorID,
	})
}

func buildTemplateFromParts(value Template) (Template, error) {
	value.Key = strings.TrimSpace(value.Key)
	value.Name = strings.TrimSpace(value.Name)
	value.Role = strings.TrimSpace(value.Role)
	value.Description = strings.TrimSpace(value.Description)
	value.Instructions = strings.TrimSpace(value.Instructions)
	if err := validateKey(value.Key); err != nil {
		return Template{}, err
	}
	if value.Name == "" || len(value.Name) > maxNameBytes {
		return Template{}, fmt.Errorf("%w: name must contain 1 to %d bytes", ErrInvalidInput, maxNameBytes)
	}
	if value.Role == "" || len(value.Role) > maxRoleBytes {
		return Template{}, fmt.Errorf("%w: role must contain 1 to %d bytes", ErrInvalidInput, maxRoleBytes)
	}
	if len(value.Description) > maxDescriptionBytes {
		return Template{}, fmt.Errorf("%w: description must not exceed %d bytes", ErrInvalidInput, maxDescriptionBytes)
	}
	if len(value.Instructions) > maxInstructionsBytes {
		return Template{}, fmt.Errorf("%w: instructions must not exceed %d bytes", ErrInvalidInput, maxInstructionsBytes)
	}
	if err := validateRuntime(value.DefaultRuntime); err != nil {
		return Template{}, err
	}
	if len(value.SuggestedTools) > maxSuggestedValues {
		return Template{}, fmt.Errorf("%w: suggested tools must contain at most %d values", ErrInvalidInput, maxSuggestedValues)
	}
	if len(value.SuggestedCapabilities) > maxSuggestedValues {
		return Template{}, fmt.Errorf("%w: suggested capabilities must contain at most %d values", ErrInvalidInput, maxSuggestedValues)
	}
	tools, err := ValidateTools(value.SuggestedTools, nil)
	if err != nil {
		return Template{}, err
	}
	capabilities, err := ValidateCapabilities(value.SuggestedCapabilities)
	if err != nil {
		return Template{}, err
	}
	value.SuggestedTools = tools
	value.SuggestedCapabilities = capabilities
	if value.Version < 1 {
		return Template{}, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	return value, nil
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
