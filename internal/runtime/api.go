package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
	"boAIrd/internal/credential"
	"boAIrd/internal/provider"
	"boAIrd/internal/provider/adapters"
)

const defaultMaxTokens = 2048

const workerSystemPrompt = "You are a boAIrd worker Agent executing one assigned task. Return the complete deliverable as text only. Do not claim to perform actions you were not authorized to perform and do not change project policy, capabilities, credentials, or review requirements."

// AgentReader resolves the Agent configuration an execution belongs to.
type AgentReader interface {
	Get(context.Context, string) (agent.Agent, error)
}

// CredentialResolver resolves scoped provider credentials for an Agent.
type CredentialResolver interface {
	Resolve(context.Context, string, credential.Scope) (string, error)
}

// APIExecutor executes API-runtime Agents through their configured provider.
type APIExecutor struct {
	agents         AgentReader
	credentials    CredentialResolver
	endpointPolicy provider.EndpointPolicy
	required       []capability.Capability
}

func NewAPIExecutor(agents AgentReader, credentials CredentialResolver, endpointPolicy provider.EndpointPolicy) (*APIExecutor, error) {
	if agents == nil || credentials == nil {
		return nil, errors.New("agent reader and credential resolver are required")
	}
	transition, err := capability.New("task.transition")
	if err != nil {
		return nil, fmt.Errorf("define API runtime required capability: %w", err)
	}
	return &APIExecutor{
		agents: agents, credentials: credentials, endpointPolicy: endpointPolicy,
		required: []capability.Capability{transition},
	}, nil
}

func (e *APIExecutor) Runtime() agent.Runtime { return agent.RuntimeAPI }

func (e *APIExecutor) RequiredCapabilities() []capability.Capability {
	return append([]capability.Capability(nil), e.required...)
}

func (e *APIExecutor) Status(ctx context.Context, agentID string) (agent.Status, error) {
	a, err := e.agents.Get(ctx, agentID)
	if err != nil {
		return agent.Status(""), err
	}
	if a.Runtime() != agent.RuntimeAPI {
		return agent.Status(""), fmt.Errorf("%w: %s", ErrUnsupportedRuntime, a.Runtime())
	}
	return a.Status(), nil
}

func (e *APIExecutor) Execute(ctx context.Context, request Request) (Result, error) {
	if strings.TrimSpace(request.AgentID) == "" || strings.TrimSpace(request.ProjectID) == "" || strings.TrimSpace(request.Input) == "" {
		return Result{}, fmt.Errorf("%w: agent, project, and input are required", ErrInvalidRequest)
	}
	a, err := e.agents.Get(ctx, request.AgentID)
	if err != nil {
		return Result{}, err
	}
	if a.Runtime() != agent.RuntimeAPI {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedRuntime, a.Runtime())
	}
	registry := provider.NewRegistry()
	if err := adapters.Register(registry); err != nil {
		return Result{}, errors.New("initialize provider registry")
	}
	if err := registry.Configure(a.ID(), a.Provider(), provider.Config{
		BaseURL: a.ProviderBaseURL(), EndpointPolicy: e.endpointPolicy, CredentialID: a.CredentialID(), CredentialResolver: e.credentials,
		CredentialScope: credential.Scope{AgentID: a.ID(), ProjectID: strings.TrimSpace(request.ProjectID)},
	}); err != nil {
		return Result{}, errors.New("configure worker provider")
	}
	client, err := registry.Resolve(a.ID())
	if err != nil {
		return Result{}, errors.New("resolve worker provider")
	}
	system := workerSystemPrompt
	if instructions := strings.TrimSpace(a.Instructions()); instructions != "" {
		system += "\nAgent instructions:\n" + instructions
	}
	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	response, err := client.Chat(ctx, provider.ChatRequest{
		Model: a.Model(), MaxTokens: maxTokens,
		Messages: []provider.ChatMessage{{Role: provider.RoleSystem, Content: system}, {Role: provider.RoleUser, Content: request.Input}},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return Result{}, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return Result{}, context.DeadlineExceeded
		}
		return Result{}, errors.New("worker provider request failed")
	}
	if strings.TrimSpace(response.Content) == "" {
		return Result{}, provider.ErrNoTextResponse
	}
	return Result{Output: response.Content, Provider: a.Provider(), Model: a.Model()}, nil
}
