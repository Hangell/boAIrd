// Package runtime defines the provider-independent contract for executing Agents.
package runtime

import (
	"context"
	"errors"
)

var (
	ErrUnsupportedRuntime = errors.New("agent runtime does not support push execution")
	ErrInvalidRequest     = errors.New("invalid runtime request")
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
