// Package tool defines an explicitly registered, policy-enforced tool system.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"boAIrd/internal/capability"
)

var (
	ErrInvalidRequest = errors.New("invalid tool request")
	ErrDenied         = errors.New("tool action denied")
	ErrNotFound       = errors.New("tool is not registered")
	ErrInvalidInput   = errors.New("tool input is invalid")
	ErrTimedOut       = errors.New("tool execution timed out")
	ErrExecution      = errors.New("tool execution failed")
	ErrAudit          = errors.New("tool audit could not be recorded")
)

const (
	DefaultMaxInputBytes  = 1 << 20
	DefaultMaxOutputBytes = 1 << 20
	maximumTimeout        = time.Hour
	auditTimeout          = 5 * time.Second
)

// Principal is a server-authenticated identity and its explicit grants.
type Principal struct {
	ID           string
	Type         string
	Capabilities []capability.Capability
	ProjectIDs   []string
}

// Request identifies the resource boundary and typed JSON input of a call.
type Request struct {
	ToolName     string
	ProjectID    string
	TaskID       string
	RunID        string
	ResourceType string
	ResourceID   string
	Principal    Principal
	Input        json.RawMessage
}

// Limits bounds input, output, and wall-clock execution for one tool.
type Limits struct {
	Timeout        time.Duration
	MaxInputBytes  int
	MaxOutputBytes int
}

// Contract is the reviewable declaration every registered tool must provide.
type Contract struct {
	Name                 string
	Description          string
	InputSchema          json.RawMessage
	OutputSchema         json.RawMessage
	RequiredCapabilities []capability.Capability
	Limits               Limits
}

// Event contains only non-sensitive metadata suitable for immutable audit.
type Event struct {
	ProjectID    string
	TaskID       string
	RunID        string
	ActorType    string
	ActorID      string
	ToolName     string
	ResourceType string
	ResourceID   string
	Outcome      string
}

// Auditor persists immutable tool execution decisions without input/output data.
type Auditor interface {
	RecordToolExecution(context.Context, Event) error
}

// ScopeAuthorizer validates the principal's project and resource boundary.
type ScopeAuthorizer interface {
	Authorize(context.Context, Principal, Request) error
}

// ScopeAuthorizerFunc adapts a function that verifies project and resource ownership.
type ScopeAuthorizerFunc func(context.Context, Principal, Request) error

func (f ScopeAuthorizerFunc) Authorize(ctx context.Context, principal Principal, request Request) error {
	return f(ctx, principal, request)
}

// Tool is the erased contract used by the explicit registry and enforcer.
type Tool interface {
	Contract() Contract
	validDefinition() bool
	frozen() Tool
	decodeAndValidate(json.RawMessage) (any, error)
	execute(context.Context, Request, any) (json.RawMessage, error)
}

type auditResourceResolver interface {
	auditResourceID(context.Context, Request, any) (string, error)
}

// Definition adapts a typed input/output tool to the registry contract.
type Definition[I, O any] struct {
	Metadata        Contract
	Validate        func(I) error
	ValidateOutput  func(O) error
	AuditResourceID func(context.Context, Request, I) (string, error)
	Run             func(context.Context, I) (O, error)
	RunWithRequest  func(context.Context, Request, I) (O, error)
}

func (d Definition[I, O]) Contract() Contract { return cloneContract(d.Metadata) }

func (d Definition[I, O]) validDefinition() bool {
	return d.Validate != nil && d.ValidateOutput != nil && ((d.Run != nil) != (d.RunWithRequest != nil))
}

func (d Definition[I, O]) frozen() Tool {
	d.Metadata = cloneContract(d.Metadata)
	return d
}

func (d Definition[I, O]) decodeAndValidate(raw json.RawMessage) (any, error) {
	var input I
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("%w: decode input", ErrInvalidInput)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: input must contain one JSON value", ErrInvalidInput)
	}
	if d.Validate != nil {
		if err := d.Validate(input); err != nil {
			return nil, fmt.Errorf("%w: validation failed", ErrInvalidInput)
		}
	}
	return input, nil
}

func (d Definition[I, O]) auditResourceID(ctx context.Context, request Request, value any) (string, error) {
	if d.AuditResourceID == nil {
		return request.ResourceID, nil
	}
	input, ok := value.(I)
	if !ok {
		return "", ErrInvalidInput
	}
	return d.AuditResourceID(ctx, request, input)
}

func (d Definition[I, O]) execute(ctx context.Context, request Request, value any) (json.RawMessage, error) {
	input, ok := value.(I)
	if !ok {
		return nil, ErrExecution
	}
	var output O
	var err error
	if d.RunWithRequest != nil {
		output, err = d.RunWithRequest(ctx, request, input)
	} else if d.Run != nil {
		output, err = d.Run(ctx, input)
	} else {
		return nil, ErrExecution
	}
	if err != nil {
		return nil, ErrExecution
	}
	if err := d.ValidateOutput(output); err != nil {
		return nil, ErrExecution
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil, ErrExecution
	}
	return encoded, nil
}

func cloneContract(value Contract) Contract {
	value.InputSchema = append(json.RawMessage(nil), value.InputSchema...)
	value.OutputSchema = append(json.RawMessage(nil), value.OutputSchema...)
	value.RequiredCapabilities = append([]capability.Capability(nil), value.RequiredCapabilities...)
	return value
}
