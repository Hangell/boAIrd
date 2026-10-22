package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound           = errors.New("agent not found")
	ErrInvalidInput       = errors.New("invalid agent input")
	ErrCredentialMismatch = errors.New("agent credential does not match configured provider")
)

const (
	defaultPageSize = 50
	maximumPageSize = 100
)

type Repository interface {
	Create(context.Context, Agent) error
	Get(context.Context, string) (Agent, error)
	List(context.Context, int, int) ([]Agent, error)
	Update(context.Context, Agent) error
}

type projectScopeRepository interface {
	AddProject(context.Context, string, string, string) error
	RemoveProject(context.Context, string, string, string) error
}

// ActivitySource derives server-owned runtime state and reports its history.
type ActivitySource interface {
	Statuses(ctx context.Context, agentIDs []string) (map[string]Observed, error)
	Events(ctx context.Context, agentID string, limit int) ([]StatusEvent, error)
}

type Manager struct {
	repository Repository
	activity   ActivitySource
}

type CreateInput struct {
	Name                string   `json:"name"`
	Avatar              string   `json:"avatar"`
	Description         string   `json:"description"`
	Role                string   `json:"role"`
	Runtime             Runtime  `json:"runtime"`
	Provider            string   `json:"provider,omitempty"`
	ProviderBaseURL     string   `json:"provider_base_url,omitempty"`
	Model               string   `json:"model,omitempty"`
	CredentialID        string   `json:"credential_id,omitempty"`
	Instructions        string   `json:"instructions"`
	LanguagePreferences []string `json:"language_preferences"`
	Capabilities        []string `json:"capabilities"`
}

type UpdateInput struct {
	Name                *string   `json:"name"`
	Avatar              *string   `json:"avatar"`
	Description         *string   `json:"description"`
	Role                *string   `json:"role"`
	Runtime             *Runtime  `json:"runtime"`
	Provider            *string   `json:"provider"`
	ProviderBaseURL     *string   `json:"provider_base_url"`
	Model               *string   `json:"model"`
	CredentialID        *string   `json:"credential_id"`
	Instructions        *string   `json:"instructions"`
	LanguagePreferences *[]string `json:"language_preferences"`
	Capabilities        *[]string `json:"capabilities"`
}

func NewManager(repository Repository, activities ...ActivitySource) (*Manager, error) {
	if repository == nil {
		return nil, errors.New("agent repository is required")
	}
	manager := &Manager{repository: repository}
	if len(activities) > 0 {
		manager.activity = activities[0]
	}
	return manager, nil
}

// Observe returns the observed runtime state for the given Agents. It returns
// an empty map when no activity source is configured.
func (m *Manager) Observe(ctx context.Context, agentIDs []string) (map[string]Observed, error) {
	if m.activity == nil || len(agentIDs) == 0 {
		return map[string]Observed{}, nil
	}
	return m.activity.Statuses(ctx, agentIDs)
}

// Events returns the recorded status transitions for one Agent.
func (m *Manager) Events(ctx context.Context, agentID string, limit int) ([]StatusEvent, error) {
	if m.activity == nil {
		return []StatusEvent{}, nil
	}
	return m.activity.Events(ctx, agentID, limit)
}

func (m *Manager) Create(ctx context.Context, input CreateInput) (Agent, error) {
	configuration := Configuration{
		Name: input.Name, Avatar: input.Avatar, Description: input.Description, Role: input.Role,
		Runtime: input.Runtime, Provider: input.Provider, ProviderBaseURL: input.ProviderBaseURL, Model: input.Model, CredentialID: input.CredentialID,
		Instructions: input.Instructions, LanguagePreferences: input.LanguagePreferences, Capabilities: input.Capabilities,
	}
	id, err := newID()
	if err != nil {
		return Agent{}, fmt.Errorf("generate agent ID: %w", err)
	}
	value, err := NewConfigured(id, configuration)
	if err != nil {
		return Agent{}, invalidInput(err)
	}
	if err := m.repository.Create(ctx, value); err != nil {
		return Agent{}, err
	}
	return value, nil
}

func (m *Manager) Get(ctx context.Context, id string) (Agent, error) {
	if strings.TrimSpace(id) == "" {
		return Agent{}, invalidInput(errors.New("agent ID is required"))
	}
	return m.repository.Get(ctx, id)
}

func (m *Manager) List(ctx context.Context, limit, offset int) ([]Agent, error) {
	if limit == 0 {
		limit = defaultPageSize
	}
	if limit < 1 || limit > maximumPageSize {
		return nil, invalidInput(fmt.Errorf("limit must be between 1 and %d", maximumPageSize))
	}
	if offset < 0 {
		return nil, invalidInput(errors.New("offset must not be negative"))
	}
	return m.repository.List(ctx, limit, offset)
}

func (m *Manager) Update(ctx context.Context, id string, input UpdateInput) (Agent, error) {
	if input.Name == nil && input.Avatar == nil && input.Description == nil && input.Role == nil && input.Runtime == nil &&
		input.Provider == nil && input.ProviderBaseURL == nil && input.Model == nil && input.CredentialID == nil && input.Instructions == nil &&
		input.LanguagePreferences == nil && input.Capabilities == nil {
		return Agent{}, invalidInput(errors.New("at least one agent field must be provided"))
	}
	current, err := m.Get(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	snapshot := current.Snapshot()
	if input.Name != nil {
		snapshot.Name = *input.Name
	}
	if input.Avatar != nil {
		snapshot.Avatar = *input.Avatar
	}
	if input.Description != nil {
		snapshot.Description = *input.Description
	}
	if input.Role != nil {
		snapshot.Role = *input.Role
	}
	if input.Runtime != nil {
		snapshot.Runtime = *input.Runtime
	}
	if input.Provider != nil {
		snapshot.Provider = *input.Provider
	}
	if input.ProviderBaseURL != nil {
		snapshot.ProviderBaseURL = *input.ProviderBaseURL
	}
	if input.Model != nil {
		snapshot.Model = *input.Model
	}
	if input.CredentialID != nil {
		snapshot.CredentialID = *input.CredentialID
	}
	if input.Instructions != nil {
		snapshot.Instructions = *input.Instructions
	}
	if input.LanguagePreferences != nil {
		snapshot.LanguagePreferences = *input.LanguagePreferences
	}
	if input.Capabilities != nil {
		snapshot.Capabilities = *input.Capabilities
	}
	if snapshot.Runtime != RuntimeAPI {
		snapshot.Provider, snapshot.ProviderBaseURL, snapshot.Model, snapshot.CredentialID = "", "", "", ""
	}
	value, err := Restore(snapshot)
	if err != nil {
		return Agent{}, invalidInput(err)
	}
	if err := m.repository.Update(ctx, value); err != nil {
		return Agent{}, err
	}
	return value, nil
}

func (m *Manager) AddProject(ctx context.Context, agentID, projectID, actorID string) error {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(actorID) == "" {
		return invalidInput(errors.New("agent, project, and actor IDs are required"))
	}
	repository, ok := m.repository.(projectScopeRepository)
	if !ok {
		return errors.New("agent project scope is unavailable")
	}
	return repository.AddProject(ctx, agentID, projectID, actorID)
}

func (m *Manager) RemoveProject(ctx context.Context, agentID, projectID, actorID string) error {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(actorID) == "" {
		return invalidInput(errors.New("agent, project, and actor IDs are required"))
	}
	repository, ok := m.repository.(projectScopeRepository)
	if !ok {
		return errors.New("agent project scope is unavailable")
	}
	return repository.RemoveProject(ctx, agentID, projectID, actorID)
}

func invalidInput(err error) error { return fmt.Errorf("%w: %w", ErrInvalidInput, err) }

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
