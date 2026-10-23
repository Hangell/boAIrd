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
	"net/url"
	"regexp"
	"strings"

	"boAIrd/internal/capability"
)

// SupportedSDKVersion is the only extension SDK contract this build accepts.
const SupportedSDKVersion = 1

var (
	ErrNotFound        = errors.New("extension not found")
	ErrAlreadyExists   = errors.New("extension already exists")
	ErrInvalidManifest = errors.New("invalid extension manifest")
	ErrInvalidInput    = errors.New("invalid extension input")
	ErrWrongStatus     = errors.New("extension is not in the required status")
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
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]*$`)

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
	for _, tool := range manifest.Tools {
		if err := ValidateToolDeclaration(tool); err != nil {
			return err
		}
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
	if !validJSONObject(tool.InputSchema) || !validJSONObject(tool.OutputSchema) {
		return fmt.Errorf("%w: tool %s input and output schemas must be JSON objects", ErrInvalidManifest, tool.Name)
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
	if parsed.User != nil {
		return fmt.Errorf("%w: http tool %s endpoint must not embed credentials", ErrInvalidManifest, tool.Name)
	}
	return nil
}

func validJSONObject(value json.RawMessage) bool {
	if len(value) == 0 {
		return false
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return false
	}
	_, ok := decoded.(map[string]any)
	return ok
}

// Repository persists extension manifests and lifecycle state.
type Repository interface {
	Create(context.Context, Extension) error
	List(context.Context) ([]Extension, error)
	Get(context.Context, string) (Extension, error)
	SetStatus(context.Context, string, Status) error
	Delete(context.Context, string) error
}

// ToolRegistrar registers and unregisters extension tools with the shared Enforcer.
type ToolRegistrar interface {
	RegisterTool(context.Context, ToolDeclaration) error
	UnregisterTool(context.Context, string)
}

// AuditWriter appends extension lifecycle events.
type AuditWriter interface {
	RecordExtensionEvent(context.Context, string, string, string) error
}

// Service applies manifest validation and lifecycle rules.
type Service struct {
	repository Repository
	registrar  ToolRegistrar
	audit      AuditWriter
}

func NewService(repository Repository, registrar ToolRegistrar, audit AuditWriter) (*Service, error) {
	if repository == nil {
		return nil, errors.New("extension repository is required")
	}
	if registrar == nil {
		return nil, errors.New("extension tool registrar is required")
	}
	if audit == nil {
		return nil, errors.New("extension audit writer is required")
	}
	return &Service{repository: repository, registrar: registrar, audit: audit}, nil
}

// Install validates and stores a pending extension, recording the lifecycle event.
func (s *Service) Install(ctx context.Context, manifest Manifest, actorID string) (Extension, error) {
	if err := ValidateManifest(manifest); err != nil {
		return Extension{}, err
	}
	value := Extension{name: manifest.Name, version: manifest.Version, status: StatusPending, tools: append([]ToolDeclaration(nil), manifest.Tools...)}
	if err := s.repository.Create(ctx, value); err != nil {
		return Extension{}, err
	}
	if err := s.audit.RecordExtensionEvent(ctx, actorID, manifest.Name, "extension.installed"); err != nil {
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
	value, err := s.repository.Get(ctx, name)
	if err != nil {
		return Extension{}, err
	}
	if value.status != StatusPending {
		return Extension{}, ErrWrongStatus
	}
	if err := s.repository.SetStatus(ctx, name, StatusActive); err != nil {
		return Extension{}, err
	}
	for _, tool := range value.tools {
		if err := s.registrar.RegisterTool(ctx, tool); err != nil {
			return Extension{}, fmt.Errorf("register extension tool %s: %w", tool.Name, err)
		}
	}
	if err := s.audit.RecordExtensionEvent(ctx, actorID, name, "extension.activated"); err != nil {
		return Extension{}, err
	}
	value.status = StatusActive
	return value, nil
}

// Deactivate removes an active extension and unregisters its tools.
func (s *Service) Deactivate(ctx context.Context, name, actorID string) error {
	value, err := s.repository.Get(ctx, name)
	if err != nil {
		return err
	}
	if value.status != StatusActive {
		return ErrWrongStatus
	}
	for _, tool := range value.tools {
		s.registrar.UnregisterTool(ctx, tool.Name)
	}
	if err := s.repository.Delete(ctx, name); err != nil {
		return err
	}
	return s.audit.RecordExtensionEvent(ctx, actorID, name, "extension.deactivated")
}

// Reconcile re-registers every active extension's tools, used at startup.
func (s *Service) Reconcile(ctx context.Context) error {
	values, err := s.repository.List(ctx)
	if err != nil {
		return err
	}
	for _, value := range values {
		if value.status != StatusActive {
			continue
		}
		for _, tool := range value.tools {
			if err := s.registrar.RegisterTool(ctx, tool); err != nil {
				return fmt.Errorf("register active extension tool %s: %w", tool.Name, err)
			}
		}
	}
	return nil
}
