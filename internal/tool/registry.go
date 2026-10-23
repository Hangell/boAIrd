package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"boAIrd/internal/capability"
)

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

// Registry contains only tools explicitly registered by application code.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry { return &Registry{tools: make(map[string]Tool)} }

func (r *Registry) Register(value Tool) error {
	if isNil(value) {
		return fmt.Errorf("%w: tool is required", ErrInvalidRequest)
	}
	if !value.validDefinition() {
		return fmt.Errorf("%w: tool must define input/output validation and execution", ErrInvalidRequest)
	}
	contract := value.Contract()
	if !toolNamePattern.MatchString(contract.Name) || strings.TrimSpace(contract.Description) == "" {
		return fmt.Errorf("%w: tool name and description are required", ErrInvalidRequest)
	}
	if !validSchema(contract.InputSchema) || !validSchema(contract.OutputSchema) {
		return fmt.Errorf("%w: tool input and output schemas must be JSON objects", ErrInvalidRequest)
	}
	if contract.Limits.Timeout <= 0 || contract.Limits.Timeout > maximumTimeout ||
		contract.Limits.MaxInputBytes <= 0 || contract.Limits.MaxOutputBytes <= 0 {
		return fmt.Errorf("%w: tool limits must be positive and timeout cannot exceed one hour", ErrInvalidRequest)
	}
	if len(contract.RequiredCapabilities) == 0 {
		return fmt.Errorf("%w: at least one capability is required", ErrInvalidRequest)
	}
	for _, required := range contract.RequiredCapabilities {
		if !required.Valid() {
			return fmt.Errorf("%w: invalid required capability", ErrInvalidRequest)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[contract.Name]; exists {
		return fmt.Errorf("%w: tool %s is already registered", ErrInvalidRequest, contract.Name)
	}
	r.tools[contract.Name] = value.frozen()
	return nil
}

func (r *Registry) Resolve(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return value, nil
}

// Enforcer authorizes and bounds every call before invoking a registered tool.
type Enforcer struct {
	registry   *Registry
	authorizer capability.Authorizer
	scope      ScopeAuthorizer
	auditor    Auditor
}

func NewEnforcer(registry *Registry, auditor Auditor, scopes ...ScopeAuthorizer) (*Enforcer, error) {
	if registry == nil || isNil(auditor) {
		return nil, errors.New("tool registry and auditor are required")
	}
	if len(scopes) != 1 || isNil(scopes[0]) {
		return nil, fmt.Errorf("%w: exactly one resource scope authorizer is required", ErrInvalidRequest)
	}
	return &Enforcer{registry: registry, scope: scopes[0], auditor: auditor}, nil
}

func (e *Enforcer) Execute(ctx context.Context, request Request) (json.RawMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidRequest)
	}
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	value, err := e.registry.Resolve(request.ToolName)
	if err != nil {
		return nil, err
	}
	contract := value.Contract()
	for _, required := range contract.RequiredCapabilities {
		principal := capability.Principal{ID: request.Principal.ID, Capabilities: request.Principal.Capabilities, ProjectIDs: request.Principal.ProjectIDs}
		if err := e.authorizer.Authorize(principal, required, request.ProjectID); err != nil {
			if e.record(ctx, request, "denied") != nil {
				return nil, ErrAudit
			}
			return nil, fmt.Errorf("%w: required capability missing", ErrDenied)
		}
	}
	if err := e.scope.Authorize(ctx, request.Principal, request); err != nil {
		if e.record(ctx, request, "denied") != nil {
			return nil, ErrAudit
		}
		return nil, fmt.Errorf("%w: resource scope rejected", ErrDenied)
	}
	if len(request.Input) == 0 || len(request.Input) > contract.Limits.MaxInputBytes {
		if e.record(ctx, request, "invalid") != nil {
			return nil, ErrAudit
		}
		return nil, fmt.Errorf("%w: input size is invalid", ErrInvalidInput)
	}
	input, err := value.decodeAndValidate(request.Input)
	if err != nil {
		if e.record(ctx, request, "invalid") != nil {
			return nil, ErrAudit
		}
		return nil, err
	}
	// Record an immutable metadata-only start event before any tool runs. A
	// side-effecting tool must never begin without a prior audit trail, so an
	// audit failure here aborts the call before execution starts.
	if err := e.record(ctx, request, "start"); err != nil {
		return nil, ErrAudit
	}
	executionCtx, cancel := context.WithTimeout(ctx, contract.Limits.Timeout)
	defer cancel()
	output, err := value.execute(executionCtx, request, input)
	if err == nil && errors.Is(executionCtx.Err(), context.DeadlineExceeded) {
		if e.record(ctx, request, "timeout") != nil {
			return nil, ErrAudit
		}
		return nil, ErrTimedOut
	}
	if err != nil {
		if errors.Is(executionCtx.Err(), context.DeadlineExceeded) {
			if e.record(ctx, request, "timeout") != nil {
				return nil, ErrAudit
			}
			return nil, ErrTimedOut
		}
		if e.record(ctx, request, "failed") != nil {
			return nil, ErrAudit
		}
		return nil, ErrExecution
	}
	if len(output) > contract.Limits.MaxOutputBytes {
		if e.record(ctx, request, "failed") != nil {
			return nil, ErrAudit
		}
		return nil, fmt.Errorf("%w: output size limit exceeded", ErrExecution)
	}
	if err := e.record(ctx, request, "success"); err != nil {
		return nil, ErrAudit
	}
	return output, nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func validateRequest(request Request) error {
	if request.ToolName == "" || strings.TrimSpace(request.ProjectID) == "" ||
		strings.TrimSpace(request.ProjectID) != request.ProjectID ||
		strings.TrimSpace(request.Principal.ID) == "" || strings.TrimSpace(request.Principal.Type) == "" {
		return fmt.Errorf("%w: tool, project, and authenticated principal are required", ErrInvalidRequest)
	}
	return nil
}

func validSchema(value json.RawMessage) bool {
	var schema map[string]any
	return json.Unmarshal(value, &schema) == nil && schema != nil
}

func (e *Enforcer) record(ctx context.Context, request Request, outcome string) error {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditTimeout)
	defer cancel()
	return e.auditor.RecordToolExecution(auditCtx, Event{
		ProjectID: request.ProjectID, ActorType: request.Principal.Type, ActorID: request.Principal.ID,
		ToolName: request.ToolName, ResourceType: request.ResourceType, ResourceID: request.ResourceID, Outcome: outcome,
	})
}
