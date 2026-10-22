package runtime

import (
	"context"
	"fmt"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
)

// UnsupportedRuntime is a fail-closed adapter for Agent runtimes that do not yet
// support push execution. MCP Agents pull work through authenticated tools, and
// local/remote execution is future work, so Execute always fails closed while
// Status still reflects the persisted Agent state.
type UnsupportedRuntime struct {
	kind   agent.Runtime
	reason string
	agents AgentReader
}

func NewMCPRuntime(agents AgentReader) (*UnsupportedRuntime, error) {
	return newUnsupportedRuntime(agent.RuntimeMCP, "MCP Agents claim work through authenticated tools; push execution is not supported", agents)
}

func NewLocalRuntime(agents AgentReader) (*UnsupportedRuntime, error) {
	return newUnsupportedRuntime(agent.RuntimeLocal, "local Agent execution is not implemented", agents)
}

func NewRemoteRuntime(agents AgentReader) (*UnsupportedRuntime, error) {
	return newUnsupportedRuntime(agent.RuntimeRemote, "remote Agent execution is not implemented", agents)
}

func newUnsupportedRuntime(kind agent.Runtime, reason string, agents AgentReader) (*UnsupportedRuntime, error) {
	if agents == nil {
		return nil, fmt.Errorf("%s runtime requires an agent reader", kind)
	}
	return &UnsupportedRuntime{kind: kind, reason: reason, agents: agents}, nil
}

func (r *UnsupportedRuntime) Runtime() agent.Runtime { return r.kind }

func (r *UnsupportedRuntime) RequiredCapabilities() []capability.Capability { return nil }

func (r *UnsupportedRuntime) Execute(context.Context, Request) (Result, error) {
	return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedRuntime, r.reason)
}

func (r *UnsupportedRuntime) Status(ctx context.Context, agentID string) (agent.Status, error) {
	a, err := r.agents.Get(ctx, agentID)
	if err != nil {
		return agent.Status(""), err
	}
	if a.Runtime() != r.kind {
		return agent.Status(""), fmt.Errorf("%w: %s", ErrUnsupportedRuntime, a.Runtime())
	}
	return a.Status(), nil
}
