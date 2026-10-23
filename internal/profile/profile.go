// Package profile defines reusable specialized agent profiles.
//
// A profile describes how an Agent should work; it never grants authority.
// Capabilities and tools are only effective when an administrator explicitly
// configures them for a project.
package profile

import (
	"errors"
	"fmt"
	"strings"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
)

var (
	ErrNotFound        = errors.New("profile override not found")
	ErrUnknownProfile  = errors.New("unknown agent profile")
	ErrInvalidInput    = errors.New("invalid profile input")
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectArchived = errors.New("project is archived")
)

// Definition is a built-in, reusable agent profile. Suggested capabilities and
// tools are hints shown to administrators; they are never granted implicitly.
type Definition struct {
	Key                   string
	Name                  string
	Description           string
	Instructions          string
	DefaultRuntime        agent.Runtime
	SuggestedTools        []string
	SuggestedCapabilities []string
}

// Builtins returns the built-in profiles in a stable order.
func Builtins() []Definition {
	return []Definition{
		{
			Key:                   "qa",
			Name:                  "QA",
			Description:           "Validates work against acceptance criteria, reports defects, and verifies fixes.",
			Instructions:          "Review the assigned task against its acceptance criteria. Report concrete, reproducible defects with evidence and suggest regression tests.",
			DefaultRuntime:        agent.RuntimeLocal,
			SuggestedTools:        []string{"filesystem"},
			SuggestedCapabilities: []string{"task.read", "task.review", "run.read", "conversation.read", "conversation.write"},
		},
		{
			Key:                   "security",
			Name:                  "Security Engineer",
			Description:           "Reviews changes for vulnerabilities and unsafe behavior.",
			Instructions:          "Analyze the requested change for security weaknesses, injection, secret exposure, and boundary violations. Never request or echo secrets.",
			DefaultRuntime:        agent.RuntimeLocal,
			SuggestedTools:        []string{"filesystem"},
			SuggestedCapabilities: []string{"task.read", "run.read", "conversation.read", "credential.read", "agent.read"},
		},
		{
			Key:                   "devops",
			Name:                  "DevOps",
			Description:           "Operates builds, tests, and deployment workflows under explicit shell and filesystem grants.",
			Instructions:          "Run only the commands and filesystem operations the administrator explicitly authorized for this project. Prefer structured command execution and bounded timeouts.",
			DefaultRuntime:        agent.RuntimeLocal,
			SuggestedTools:        []string{"shell", "filesystem", "git"},
			SuggestedCapabilities: []string{"task.read", "run.read", "shell.execute", "filesystem.read"},
		},
		{
			Key:                   "git-manager",
			Name:                  "Git Manager",
			Description:           "Coordinates branches, commits, and merges through granular Git operations.",
			Instructions:          "Apply granular Git operations only as explicitly authorized. Never force push, rewrite history, or delete branches without administrator approval.",
			DefaultRuntime:        agent.RuntimeLocal,
			SuggestedTools:        []string{"git"},
			SuggestedCapabilities: []string{"task.read", "task.review", "run.read"},
		},
		{
			Key:                   "architect",
			Name:                  "Software Architect",
			Description:           "Designs system structure and documents architecture decisions.",
			Instructions:          "Produce architecture and design artifacts. Keep decisions explicit and traceable; diagram tools are available only when configured.",
			DefaultRuntime:        agent.RuntimeLocal,
			SuggestedTools:        []string{"mermaid", "plantuml", "drawio"},
			SuggestedCapabilities: []string{"task.read", "conversation.read", "conversation.write"},
		},
		{
			Key:                   "product-owner",
			Name:                  "Product Owner",
			Description:           "Owns backlog scope and acceptance criteria.",
			Instructions:          "Refine backlog items and acceptance criteria. Task creation and assignment remain subject to explicit capabilities and project policy.",
			DefaultRuntime:        agent.RuntimeLocal,
			SuggestedTools:        []string{},
			SuggestedCapabilities: []string{"task.create", "task.read", "task.assign", "task.review"},
		},
		{
			Key:                   "documentation",
			Name:                  "Documentation",
			Description:           "Writes and maintains project documentation.",
			Instructions:          "Produce clear, accurate documentation. Filesystem writes are allowed only when explicitly configured for this project.",
			DefaultRuntime:        agent.RuntimeLocal,
			SuggestedTools:        []string{"filesystem"},
			SuggestedCapabilities: []string{"task.read", "conversation.read", "filesystem.read"},
		},
		{
			Key:                   "sdm",
			Name:                  "Software Delivery Manager",
			Description:           "Coordinates delivery across workers and tracks progress.",
			Instructions:          "Track task progress and coordinate workers. Assignment and review remain subject to explicit capabilities and project policy.",
			DefaultRuntime:        agent.RuntimeLocal,
			SuggestedTools:        []string{},
			SuggestedCapabilities: []string{"task.create", "task.read", "task.assign", "task.review", "agent.read"},
		},
	}
}

// Lookup returns the built-in definition for a profile key.
func Lookup(key string) (Definition, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, definition := range Builtins() {
		if definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}

// ValidCapabilities validates and deduplicates a capability list.
func ValidCapabilities(values []string) ([]string, error) {
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

// ValidateTools checks that every tool is one of the profile's suggested tools.
func (d Definition) ValidateTools(values []string) ([]string, error) {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	allowed := make(map[string]struct{}, len(d.SuggestedTools))
	for _, tool := range d.SuggestedTools {
		allowed[tool] = struct{}{}
	}
	for _, raw := range values {
		tool := strings.TrimSpace(strings.ToLower(raw))
		if tool == "" {
			return nil, fmt.Errorf("%w: tool name is required", ErrInvalidInput)
		}
		if _, ok := allowed[tool]; !ok {
			return nil, fmt.Errorf("%w: tool %q is not available for profile %q", ErrInvalidInput, tool, d.Key)
		}
		if _, exists := seen[tool]; exists {
			continue
		}
		seen[tool] = struct{}{}
		cleaned = append(cleaned, tool)
	}
	return cleaned, nil
}
