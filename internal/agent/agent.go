// Package agent defines provider-independent agent configuration and state.
package agent

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"boAIrd/internal/capability"
)

type Status string
type Runtime string

const (
	StatusOnline  Status = "ONLINE"
	StatusIdle    Status = "IDLE"
	StatusBusy    Status = "BUSY"
	StatusWaiting Status = "WAITING"
	StatusOffline Status = "OFFLINE"
	StatusStale   Status = "STALE"
	StatusError   Status = "ERROR"

	RuntimeAPI    Runtime = "API"
	RuntimeMCP    Runtime = "MCP"
	RuntimeLocal  Runtime = "LOCAL"
	RuntimeRemote Runtime = "REMOTE"
)

type Agent struct {
	id                  string
	name                string
	avatar              string
	description         string
	role                string
	runtime             Runtime
	provider            string
	model               string
	credentialID        string
	instructions        string
	languagePreferences []string
	status              Status
	capabilities        []capability.Capability
	projectIDs          []string
	tools               []string
}

type Configuration struct {
	Name                string
	Avatar              string
	Description         string
	Role                string
	Runtime             Runtime
	Provider            string
	Model               string
	CredentialID        string
	Instructions        string
	LanguagePreferences []string
	Capabilities        []string
}

type Snapshot struct {
	ID                  string
	Name                string
	Avatar              string
	Description         string
	Role                string
	Runtime             Runtime
	Provider            string
	Model               string
	CredentialID        string
	Instructions        string
	LanguagePreferences []string
	Capabilities        []string
	ProjectIDs          []string
	Tools               []string
	Status              Status
}

func New(id, name, role string) (Agent, error) {
	id, name, role = strings.TrimSpace(id), strings.TrimSpace(name), strings.TrimSpace(role)
	if id == "" || name == "" || role == "" {
		return Agent{}, errors.New("agent ID, name, and role are required")
	}
	if err := validateText(name, "name", 200); err != nil {
		return Agent{}, err
	}
	if err := validateText(role, "role", 100); err != nil {
		return Agent{}, err
	}
	return Agent{id: id, name: name, role: role, status: StatusIdle}, nil
}

func NewConfigured(id string, configuration Configuration) (Agent, error) {
	a, err := New(id, configuration.Name, configuration.Role)
	if err != nil {
		return Agent{}, err
	}
	return a.Configure(configuration)
}

func Restore(snapshot Snapshot) (Agent, error) {
	configuration := Configuration{
		Name: snapshot.Name, Avatar: snapshot.Avatar, Description: snapshot.Description, Role: snapshot.Role,
		Runtime: snapshot.Runtime, Provider: snapshot.Provider, Model: snapshot.Model,
		CredentialID: snapshot.CredentialID, Instructions: snapshot.Instructions,
		LanguagePreferences: snapshot.LanguagePreferences, Capabilities: snapshot.Capabilities,
	}
	a, err := NewConfigured(snapshot.ID, configuration)
	if err != nil {
		return Agent{}, err
	}
	if !validStatus(snapshot.Status) {
		return Agent{}, fmt.Errorf("unknown agent status %q", snapshot.Status)
	}
	a.status = snapshot.Status
	a.projectIDs = cleanStrings(snapshot.ProjectIDs)
	a.tools = cleanStrings(snapshot.Tools)
	return a, nil
}

func (a Agent) Configure(configuration Configuration) (Agent, error) {
	configuration.Name = strings.TrimSpace(configuration.Name)
	configuration.Role = strings.TrimSpace(configuration.Role)
	configuration.Avatar = strings.TrimSpace(configuration.Avatar)
	configuration.Description = strings.TrimSpace(configuration.Description)
	configuration.Runtime = Runtime(strings.ToUpper(strings.TrimSpace(string(configuration.Runtime))))
	configuration.Provider = strings.ToLower(strings.TrimSpace(configuration.Provider))
	configuration.Model = strings.TrimSpace(configuration.Model)
	configuration.CredentialID = strings.TrimSpace(configuration.CredentialID)
	configuration.Instructions = strings.TrimSpace(configuration.Instructions)
	if err := validateText(configuration.Name, "name", 200); err != nil {
		return Agent{}, err
	}
	if err := validateText(configuration.Role, "role", 100); err != nil {
		return Agent{}, err
	}
	if err := validateOptionalText(configuration.Avatar, "avatar", 1000); err != nil {
		return Agent{}, err
	}
	if err := validateOptionalText(configuration.Description, "description", 4000); err != nil {
		return Agent{}, err
	}
	if err := validateOptionalText(configuration.Instructions, "instructions", 16000); err != nil {
		return Agent{}, err
	}
	if !validRuntime(configuration.Runtime) {
		return Agent{}, fmt.Errorf("unknown agent runtime %q", configuration.Runtime)
	}
	if err := validateOptionalText(configuration.Provider, "provider", 100); err != nil {
		return Agent{}, err
	}
	if err := validateOptionalText(configuration.Model, "model", 200); err != nil {
		return Agent{}, err
	}
	if err := validateOptionalText(configuration.CredentialID, "credential_id", 200); err != nil {
		return Agent{}, err
	}
	if configuration.Runtime == RuntimeAPI {
		if configuration.Provider == "" || configuration.Model == "" || configuration.CredentialID == "" {
			return Agent{}, errors.New("API runtime requires provider, model, and credential_id")
		}
	} else if configuration.Provider != "" || configuration.Model != "" || configuration.CredentialID != "" {
		return Agent{}, errors.New("provider configuration is only valid for the API runtime")
	}
	languages, err := validateStringList(configuration.LanguagePreferences, "language_preferences", 20, 100)
	if err != nil {
		return Agent{}, err
	}
	capabilities := make([]capability.Capability, 0, len(configuration.Capabilities))
	seen := make(map[string]struct{}, len(configuration.Capabilities))
	if len(configuration.Capabilities) > 100 {
		return Agent{}, errors.New("capabilities must contain at most 100 values")
	}
	for _, raw := range configuration.Capabilities {
		value, err := capability.New(raw)
		if err != nil {
			return Agent{}, fmt.Errorf("invalid capability: %w", err)
		}
		if _, exists := seen[string(value)]; exists {
			continue
		}
		seen[string(value)] = struct{}{}
		capabilities = append(capabilities, value)
	}
	a.name, a.avatar, a.description, a.role = configuration.Name, configuration.Avatar, configuration.Description, configuration.Role
	a.runtime, a.provider, a.model, a.credentialID = configuration.Runtime, configuration.Provider, configuration.Model, configuration.CredentialID
	a.instructions, a.languagePreferences, a.capabilities = configuration.Instructions, languages, capabilities
	return a, nil
}

func (a Agent) ID() string                    { return a.id }
func (a Agent) Name() string                  { return a.name }
func (a Agent) Avatar() string                { return a.avatar }
func (a Agent) Description() string           { return a.description }
func (a Agent) Role() string                  { return a.role }
func (a Agent) Runtime() Runtime              { return a.runtime }
func (a Agent) Provider() string              { return a.provider }
func (a Agent) Model() string                 { return a.model }
func (a Agent) CredentialID() string          { return a.credentialID }
func (a Agent) Instructions() string          { return a.instructions }
func (a Agent) Status() Status                { return a.status }
func (a Agent) LanguagePreferences() []string { return append([]string(nil), a.languagePreferences...) }

func (a Agent) Capabilities() []capability.Capability {
	return append([]capability.Capability(nil), a.capabilities...)
}

func (a Agent) ProjectIDs() []string { return append([]string(nil), a.projectIDs...) }
func (a Agent) Tools() []string      { return append([]string(nil), a.tools...) }

func (a Agent) Snapshot() Snapshot {
	capabilities := make([]string, len(a.capabilities))
	for i, value := range a.capabilities {
		capabilities[i] = string(value)
	}
	return Snapshot{
		ID: a.id, Name: a.name, Avatar: a.avatar, Description: a.description, Role: a.role,
		Runtime: a.runtime, Provider: a.provider, Model: a.model, CredentialID: a.credentialID,
		Instructions: a.instructions, LanguagePreferences: append([]string{}, a.languagePreferences...),
		Capabilities: capabilities, ProjectIDs: append([]string(nil), a.projectIDs...), Tools: append([]string(nil), a.tools...), Status: a.status,
	}
}

func (a Agent) SetStatus(status Status) (Agent, error) {
	if !validStatus(status) {
		return Agent{}, fmt.Errorf("unknown agent status %q", status)
	}
	a.status = status
	return a, nil
}

func (a Agent) Rename(name string) (Agent, error) {
	name = strings.TrimSpace(name)
	if err := validateText(name, "name", 200); err != nil {
		return Agent{}, err
	}
	a.name = name
	return a, nil
}

func (a Agent) SetRole(role string) (Agent, error) {
	role = strings.TrimSpace(role)
	if err := validateText(role, "role", 100); err != nil {
		return Agent{}, err
	}
	a.role = role
	return a, nil
}

func (a Agent) AddProject(projectID string) (Agent, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Agent{}, errors.New("project ID is required")
	}
	a.projectIDs = appendUnique(a.projectIDs, projectID)
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
	a.tools = appendUnique(a.tools, name)
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

func validRuntime(runtime Runtime) bool {
	switch runtime {
	case RuntimeAPI, RuntimeMCP, RuntimeLocal, RuntimeRemote:
		return true
	default:
		return false
	}
}

func validateText(value, field string, maximum int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("agent %s is required", field)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("agent %s must not contain NUL", field)
	}
	if utf8.RuneCountInString(value) > maximum {
		return fmt.Errorf("agent %s must be at most %d characters", field, maximum)
	}
	return nil
}

func validateOptionalText(value, field string, maximum int) error {
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("agent %s must not contain NUL", field)
	}
	if utf8.RuneCountInString(value) > maximum {
		return fmt.Errorf("agent %s must be at most %d characters", field, maximum)
	}
	return nil
}

func validateStringList(values []string, field string, maxCount, maxLength int) ([]string, error) {
	if len(values) > maxCount {
		return nil, fmt.Errorf("%s must contain at most %d values", field, maxCount)
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if err := validateText(value, field+" item", maxLength); err != nil {
			return nil, err
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = appendUnique(result, strings.TrimSpace(value))
		}
	}
	return result
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(append([]string(nil), values...), value)
}
