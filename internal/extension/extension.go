// Package extension defines the governed, declarative tool extension system.
//
// Extensions are reviewed manifests, never executable code. They declare tool
// contracts (inputs, outputs, capabilities, data access, and trust
// requirements) and an administrator approves and activates them. Activation
// registers the declared tools with the shared tool Enforcer so every
// invocation is capability- and resource-checked; untrusted extensions cannot
// read arbitrary files or secrets and cannot execute arbitrary programs.
package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"boAIrd/internal/capability"
)

// SupportedSDKVersion is the only extension SDK contract this build accepts.
const SupportedSDKVersion = 1

var (
	ErrNotFound         = errors.New("extension not found")
	ErrAlreadyExists    = errors.New("extension already exists")
	ErrInvalidManifest  = errors.New("invalid extension manifest")
	ErrInvalidInput     = errors.New("invalid extension input")
	ErrWrongStatus      = errors.New("extension is not in the required status")
	ErrToolNameConflict = errors.New("extension tool name is already reserved")
)

type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
)

type ToolKind string

const (
	KindDeclarative ToolKind = "declarative"
	KindHTTP        ToolKind = "http"
)

var extensionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

// ToolDeclaration is the reviewable contract for one extension tool.
type ToolDeclaration struct {
	Name                 string                  `json:"name"`
	Description          string                  `json:"description"`
	Kind                 ToolKind                `json:"kind"`
	Endpoint             string                  `json:"endpoint,omitempty"`
	InputSchema          json.RawMessage         `json:"input_schema"`
	OutputSchema         json.RawMessage         `json:"output_schema"`
	RequiredCapabilities []capability.Capability `json:"required_capabilities"`
	DataAccess           string                  `json:"data_access,omitempty"`
}

// Manifest is the versioned, validated extension declaration.
type Manifest struct {
	SDKVersion int               `json:"sdk_version"`
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Tools      []ToolDeclaration `json:"tools"`
}

// Extension is a stored manifest plus its lifecycle status.
type Extension struct {
	name    string
	version string
	status  Status
	tools   []ToolDeclaration
}

func (e Extension) Name() string             { return e.name }
func (e Extension) Version() string          { return e.version }
func (e Extension) Status() Status           { return e.status }
func (e Extension) Tools() []ToolDeclaration { return append([]ToolDeclaration(nil), e.tools...) }

// Restore rebuilds an Extension from persisted fields.
func Restore(name, version string, status Status, tools []ToolDeclaration) (Extension, error) {
	name = strings.TrimSpace(name)
	if !extensionNamePattern.MatchString(name) {
		return Extension{}, ErrInvalidManifest
	}
	if strings.TrimSpace(version) == "" || len(version) > 64 {
		return Extension{}, ErrInvalidManifest
	}
	if status != StatusPending && status != StatusActive {
		return Extension{}, ErrInvalidManifest
	}
	return Extension{name: name, version: version, status: status, tools: append([]ToolDeclaration(nil), tools...)}, nil
}

// ValidateManifest checks the compatibility contract and every tool declaration.
func ValidateManifest(manifest Manifest) error {
	if manifest.SDKVersion != SupportedSDKVersion {
		return fmt.Errorf("%w: unsupported sdk_version %d (want %d)", ErrInvalidManifest, manifest.SDKVersion, SupportedSDKVersion)
	}
	if !extensionNamePattern.MatchString(manifest.Name) || len(manifest.Name) > 64 {
		return fmt.Errorf("%w: extension name must match %s", ErrInvalidManifest, extensionNamePattern.String())
	}
	if strings.TrimSpace(manifest.Version) == "" || len(manifest.Version) > 64 || strings.ContainsRune(manifest.Version, '\x00') {
		return fmt.Errorf("%w: extension version is required and bounded", ErrInvalidManifest)
	}
	if len(manifest.Tools) == 0 || len(manifest.Tools) > 16 {
		return fmt.Errorf("%w: extension must declare between 1 and 16 tools", ErrInvalidManifest)
	}
	seen := make(map[string]struct{}, len(manifest.Tools))
	for _, tool := range manifest.Tools {
		if err := ValidateToolDeclaration(tool); err != nil {
			return err
		}
		if _, exists := seen[tool.Name]; exists {
			return fmt.Errorf("%w: duplicate tool name %q", ErrInvalidManifest, tool.Name)
		}
		seen[tool.Name] = struct{}{}
	}
	return nil
}

// ValidateToolDeclaration validates one tool contract and its trust boundary.
func ValidateToolDeclaration(tool ToolDeclaration) error {
	if !toolNamePattern.MatchString(tool.Name) || len(tool.Name) > 64 {
		return fmt.Errorf("%w: tool name must match %s", ErrInvalidManifest, toolNamePattern.String())
	}
	if strings.TrimSpace(tool.Description) == "" {
		return fmt.Errorf("%w: tool %s description is required", ErrInvalidManifest, tool.Name)
	}
	if tool.Kind != KindDeclarative && tool.Kind != KindHTTP {
		return fmt.Errorf("%w: tool %s kind must be declarative or http", ErrInvalidManifest, tool.Name)
	}
	if !validSchema(tool.InputSchema) || !validSchema(tool.OutputSchema) {
		return fmt.Errorf("%w: tool %s input and output schemas are invalid or unsupported", ErrInvalidManifest, tool.Name)
	}
	if len(tool.RequiredCapabilities) == 0 {
		return fmt.Errorf("%w: tool %s must declare at least one capability", ErrInvalidManifest, tool.Name)
	}
	for _, required := range tool.RequiredCapabilities {
		if !required.Valid() {
			return fmt.Errorf("%w: tool %s declares an invalid capability", ErrInvalidManifest, tool.Name)
		}
	}
	if err := validateDataAccess(tool); err != nil {
		return err
	}
	if tool.Kind == KindHTTP {
		if err := validateHTTPEndpoint(tool); err != nil {
			return err
		}
	}
	return nil
}

// validateDataAccess rejects any declaration that could read files, secrets, or
// execute programs. Extensions are allowed no data access or bounded network access.
func validateDataAccess(tool ToolDeclaration) error {
	access := strings.TrimSpace(tool.DataAccess)
	if access == "" {
		access = "none"
	}
	lower := strings.ToLower(access)
	switch {
	case lower == "none":
		return nil
	case strings.HasPrefix(lower, "network:"):
		host := strings.TrimPrefix(lower, "network:")
		if host == "" || strings.ContainsAny(host, " \t\r\n/\\") {
			return fmt.Errorf("%w: tool %s data_access network host is invalid", ErrInvalidManifest, tool.Name)
		}
		return nil
	default:
		return fmt.Errorf("%w: tool %s data_access %q is not allowed; extensions cannot read files, secrets, or execute programs", ErrInvalidManifest, tool.Name, tool.DataAccess)
	}
}

func validateHTTPEndpoint(tool ToolDeclaration) error {
	if tool.Kind != KindHTTP {
		return nil
	}
	if tool.Endpoint == "" {
		return fmt.Errorf("%w: http tool %s requires an endpoint", ErrInvalidManifest, tool.Name)
	}
	parsed, err := url.ParseRequestURI(tool.Endpoint)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return fmt.Errorf("%w: http tool %s endpoint must be an absolute HTTPS URL", ErrInvalidManifest, tool.Name)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return fmt.Errorf("%w: http tool %s endpoint must not contain credentials, query, or fragment data", ErrInvalidManifest, tool.Name)
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return fmt.Errorf("%w: http tool %s endpoint must use the standard HTTPS port", ErrInvalidManifest, tool.Name)
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if !validExtensionHost(host) || strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil {
		return fmt.Errorf("%w: http tool %s endpoint must use an exact public DNS hostname", ErrInvalidManifest, tool.Name)
	}
	access := strings.TrimSpace(tool.DataAccess)
	if !strings.EqualFold(access, "network:"+host) {
		return fmt.Errorf("%w: http tool %s data_access must match the endpoint host", ErrInvalidManifest, tool.Name)
	}
	return nil
}

func validExtensionHost(host string) bool {
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "/@?#\\:% \t\r\n") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if character < 'a' || character > 'z' {
				if (character < '0' || character > '9') && character != '-' {
					return false
				}
			}
		}
	}
	return true
}

// Repository persists extension manifests and lifecycle state.
type Repository interface {
	List(context.Context) ([]Extension, error)
	Get(context.Context, string) (Extension, error)
	IsActiveTool(context.Context, string) (bool, error)
	InstallWithAudit(context.Context, Extension, string) error
	ActivateWithAudit(context.Context, string, string) error
	DeactivateWithAudit(context.Context, string, string) error
}

// ToolRegistrar atomically registers and unregisters an extension's tools.
type ToolRegistrar interface {
	RegisterTools(context.Context, []ToolDeclaration) error
	UnregisterTools(context.Context, []string)
}

// Service applies manifest validation and lifecycle rules.
type Service struct {
	repository Repository
	registrar  ToolRegistrar
	mu         sync.Mutex
}

func NewService(repository Repository, registrar ToolRegistrar) (*Service, error) {
	if repository == nil {
		return nil, errors.New("extension repository is required")
	}
	if registrar == nil {
		return nil, errors.New("extension tool registrar is required")
	}
	return &Service{repository: repository, registrar: registrar}, nil
}

// Install validates and stores a pending extension, recording the lifecycle event.
func (s *Service) Install(ctx context.Context, manifest Manifest, actorID string) (Extension, error) {
	if err := ValidateManifest(manifest); err != nil {
		return Extension{}, err
	}
	value := Extension{name: manifest.Name, version: manifest.Version, status: StatusPending, tools: append([]ToolDeclaration(nil), manifest.Tools...)}
	if err := s.repository.InstallWithAudit(ctx, value, actorID); err != nil {
		return Extension{}, err
	}
	return value, nil
}

func (s *Service) List(ctx context.Context) ([]Extension, error) {
	return s.repository.List(ctx)
}

func (s *Service) Get(ctx context.Context, name string) (Extension, error) {
	return s.repository.Get(ctx, name)
}

// Activate moves a pending extension to active and registers its tools.
func (s *Service) Activate(ctx context.Context, name, actorID string) (Extension, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.repository.Get(ctx, name)
	if err != nil {
		return Extension{}, err
	}
	if value.status != StatusPending {
		return Extension{}, ErrWrongStatus
	}
	if err := s.registrar.RegisterTools(ctx, value.Tools()); err != nil {
		return Extension{}, fmt.Errorf("register extension tools: %w", err)
	}
	if err := s.repository.ActivateWithAudit(ctx, name, actorID); err != nil {
		s.registrar.UnregisterTools(ctx, toolNames(value.Tools()))
		return Extension{}, fmt.Errorf("activate extension: %w", err)
	}
	value.status = StatusActive
	return value, nil
}

// Deactivate removes an active extension and unregisters its tools.
func (s *Service) Deactivate(ctx context.Context, name, actorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.repository.Get(ctx, name)
	if err != nil {
		return err
	}
	if value.status != StatusActive {
		return ErrWrongStatus
	}
	if err := s.repository.DeactivateWithAudit(ctx, name, actorID); err != nil {
		return err
	}
	s.registrar.UnregisterTools(ctx, toolNames(value.Tools()))
	return nil
}

// Reconcile re-registers every active extension's tools, used at startup.
func (s *Service) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.repository.List(ctx)
	if err != nil {
		return err
	}
	registered := make([]string, 0)
	for _, value := range values {
		if value.status != StatusActive {
			continue
		}
		if err := s.registrar.RegisterTools(ctx, value.Tools()); err != nil {
			s.registrar.UnregisterTools(ctx, registered)
			return fmt.Errorf("register active extension %s: %w", value.Name(), err)
		}
		registered = append(registered, toolNames(value.Tools())...)
	}
	return nil
}

func toolNames(tools []ToolDeclaration) []string {
	names := make([]string, 0, len(tools))
	for _, value := range tools {
		names = append(names, value.Name)
	}
	return names
}
