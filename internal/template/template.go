// Package template defines reusable agent templates and their project
// instances. A template describes how an Agent should be configured; it never
// grants capabilities implicitly.
package template

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
)

var (
	ErrNotFound        = errors.New("agent template not found")
	ErrInvalidInput    = errors.New("invalid agent template input")
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectArchived = errors.New("project is archived")
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

// Template is a reusable, versioned agent definition.
type Template struct {
	ID                    string
	Key                   string
	Name                  string
	Role                  string
	Description           string
	Instructions          string
	DefaultRuntime        agent.Runtime
	SuggestedTools        []string
	SuggestedCapabilities []string
	Version               int
	CreatedBy             string
	UpdatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Instance records the template source and version for one instantiated Agent.
type Instance struct {
	AgentID         string
	ProjectID       string
	TemplateKey     string
	TemplateVersion int
	CreatedAt       time.Time
}

// AuditEvent describes a template mutation for immutable audit.
type AuditEvent struct {
	ActorID     string
	Action      string // created, updated, instantiated
	TemplateKey string
	ProjectID   string
	Version     int
}

func validateKey(key string) error {
	key = strings.TrimSpace(key)
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("%w: template key must be 2-64 lowercase letters, digits, or hyphens", ErrInvalidInput)
	}
	return nil
}

func validateRuntime(runtime agent.Runtime) error {
	switch runtime {
	case agent.RuntimeAPI, agent.RuntimeMCP, agent.RuntimeLocal, agent.RuntimeRemote:
		return nil
	default:
		return fmt.Errorf("%w: unknown agent runtime %q", ErrInvalidInput, runtime)
	}
}

// ValidateCapabilities validates and deduplicates a capability list.
func ValidateCapabilities(values []string) ([]string, error) {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value, err := capability.New(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		key := string(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, key)
	}
	return cleaned, nil
}

// ValidateTools validates and deduplicates a tool list against the allowed set.
func ValidateTools(values, allowed []string) ([]string, error) {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	allow := make(map[string]struct{}, len(allowed))
	for _, tool := range allowed {
		allow[strings.ToLower(strings.TrimSpace(tool))] = struct{}{}
	}
	for _, raw := range values {
		tool := strings.ToLower(strings.TrimSpace(raw))
		if tool == "" {
			return nil, fmt.Errorf("%w: tool name is required", ErrInvalidInput)
		}
		if len(allow) > 0 {
			if _, ok := allow[tool]; !ok {
				return nil, fmt.Errorf("%w: tool %q is not suggested by the template", ErrInvalidInput, tool)
			}
		}
		if _, exists := seen[tool]; exists {
			continue
		}
		seen[tool] = struct{}{}
		cleaned = append(cleaned, tool)
	}
	return cleaned, nil
}
