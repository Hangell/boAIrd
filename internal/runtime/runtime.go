// Package runtime defines the provider-independent contract for executing Agents.
package runtime

import (
	"context"
	"errors"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
)

var (
	ErrUnsupportedRuntime = errors.New("agent runtime does not support push execution")
	ErrInvalidRequest     = errors.New("invalid runtime request")
	ErrDenied             = errors.New("agent runtime action is not authorized")
	ErrTimeout            = errors.New("agent runtime execution timed out")
	ErrCancelled          = errors.New("agent runtime execution cancelled")
	ErrFailed             = errors.New("agent runtime execution failed")
	ErrUnknownRuntime     = errors.New("agent runtime is not registered")
)

// Request describes one bounded Agent execution owned by a Run.
type Request struct {
	RunID     string
	ProjectID string
	TaskID    string
	AgentID   string
	Input     string
	MaxTokens int
}

// Result contains the deliverable produced by an Agent execution.
type Result struct {
	Output   string
	Provider string
	Model    string
}

// Executor performs one Agent execution within the caller's cancellation and deadline.
type Executor interface {
	Execute(context.Context, Request) (Result, error)
}

// AgentRuntime is a provider-independent adapter for one Agent runtime kind.
// Domain services depend only on this contract; runtime-specific behavior stays
// inside the adapters.
type AgentRuntime interface {
	Executor

	// Runtime reports which agent.Runtime this adapter serves.
	Runtime() agent.Runtime

	// RequiredCapabilities returns the capabilities an Agent must hold before
	// this runtime executes it.
	RequiredCapabilities() []capability.Capability

	// Status resolves the Agent's current status.
	Status(context.Context, string) (agent.Status, error)
}
