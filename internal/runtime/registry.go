package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
)

// Registry resolves Agent runtime adapters by their runtime kind.
type Registry struct {
	mu       sync.RWMutex
	adapters map[agent.Runtime]AgentRuntime
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[agent.Runtime]AgentRuntime)}
}

func (r *Registry) Register(kind agent.Runtime, adapter AgentRuntime) error {
	if strings.TrimSpace(string(kind)) == "" {
		return fmt.Errorf("%w: runtime kind is required", ErrInvalidRequest)
	}
	if adapter == nil {
		return fmt.Errorf("%w: runtime adapter is required", ErrInvalidRequest)
	}
	if adapter.Runtime() != kind {
		return fmt.Errorf("%w: adapter serves %s, not %s", ErrInvalidRequest, adapter.Runtime(), kind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[kind]; exists {
		return fmt.Errorf("%w: runtime %s is already registered", ErrInvalidRequest, kind)
	}
	r.adapters[kind] = adapter
	return nil
}

func (r *Registry) Resolve(kind agent.Runtime) (AgentRuntime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, exists := r.adapters[kind]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnknownRuntime, kind)
	}
	return adapter, nil
}

// Enforcer applies the shared policy around every resolved runtime: Agent
// resolution, runtime kind, declared capabilities, a bounded deadline, and
// uniform error mapping. It implements Executor for domain services.
type Enforcer struct {
	registry *Registry
	agents   AgentReader
	timeout  time.Duration
}

func NewEnforcer(registry *Registry, agents AgentReader, timeout time.Duration) (*Enforcer, error) {
	if registry == nil || agents == nil {
		return nil, errors.New("runtime registry and agent reader are required")
	}
	if timeout < 0 {
		return nil, fmt.Errorf("%w: timeout must be non-negative", ErrInvalidRequest)
	}
	return &Enforcer{registry: registry, agents: agents, timeout: timeout}, nil
}

func (e *Enforcer) Execute(ctx context.Context, request Request) (Result, error) {
	a, err := e.agents.Get(ctx, request.AgentID)
	if err != nil {
		return Result{}, err
	}
	adapter, err := e.registry.Resolve(a.Runtime())
	if err != nil {
		return Result{}, err
	}
	for _, required := range adapter.RequiredCapabilities() {
		if !hasCapability(a, required) {
			return Result{}, fmt.Errorf("%w: %s", ErrDenied, required)
		}
	}
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}
	result, err := adapter.Execute(ctx, request)
	return result, mapExecutionError(err)
}

// Status resolves an Agent's status through its registered runtime adapter.
func (e *Enforcer) Status(ctx context.Context, agentID string) (agent.Status, error) {
	a, err := e.agents.Get(ctx, agentID)
	if err != nil {
		return agent.Status(""), err
	}
	adapter, err := e.registry.Resolve(a.Runtime())
	if err != nil {
		return agent.Status(""), err
	}
	return adapter.Status(ctx, agentID)
}

func hasCapability(a agent.Agent, required capability.Capability) bool {
	for _, value := range a.Capabilities() {
		if value == required {
			return true
		}
	}
	return false
}

// mapExecutionError normalizes adapter failures so callers can branch on shared
// sentinels while preserving the underlying cancellation/deadline causes.
func mapExecutionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%w: %w", ErrCancelled, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", ErrTimeout, context.DeadlineExceeded)
	case errors.Is(err, ErrDenied), errors.Is(err, ErrUnsupportedRuntime),
		errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrUnknownRuntime):
		return err
	default:
		return fmt.Errorf("%w: %v", ErrFailed, err)
	}
}
