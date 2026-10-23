package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"boAIrd/internal/extension"
	"boAIrd/internal/logging"
	"boAIrd/internal/provider"
	governedtool "boAIrd/internal/tool"
)

const (
	extensionHTTPTimeout     = 30 * time.Second
	extensionHTTPMaxResponse = 256 << 10
)

type extensionToolRegistrar struct {
	registry *governedtool.Registry
	client   *http.Client
	policy   provider.EndpointPolicy
}

func newExtensionToolRegistrar(registry *governedtool.Registry, policy provider.EndpointPolicy) *extensionToolRegistrar {
	return &extensionToolRegistrar{
		registry: registry,
		client:   policy.NewExtensionHTTPClient(extensionHTTPTimeout),
		policy:   policy,
	}
}

func (r *extensionToolRegistrar) RegisterTools(_ context.Context, declarations []extension.ToolDeclaration) error {
	tools := make([]governedtool.Tool, 0, len(declarations))
	for _, declaration := range declarations {
		if declaration.Kind == extension.KindDeclarative {
			continue
		}
		if declaration.Kind != extension.KindHTTP || r.policy.ValidateExtensionEndpoint(declaration.Endpoint) != nil {
			return fmt.Errorf("%w: extension endpoint is not approved", extension.ErrInvalidManifest)
		}
		value := declaration
		tool := governedtool.Definition[json.RawMessage, json.RawMessage]{
			Metadata: governedtool.Contract{
				Name: value.Name, Description: value.Description,
				InputSchema: value.InputSchema, OutputSchema: value.OutputSchema,
				RequiredCapabilities: value.RequiredCapabilities,
				Limits:               governedtool.Limits{Timeout: extensionHTTPTimeout, MaxInputBytes: 64 << 10, MaxOutputBytes: extensionHTTPMaxResponse + 4096},
			},
			Validate: func(input json.RawMessage) error {
				return extension.ValidateJSONSchema(value.InputSchema, input)
			},
			ValidateOutput: func(output json.RawMessage) error {
				return extension.ValidateJSONSchema(value.OutputSchema, output)
			},
			RunWithRequest: func(ctx context.Context, _ governedtool.Request, input json.RawMessage) (json.RawMessage, error) {
				return r.runHTTP(ctx, value, input)
			},
		}
		tools = append(tools, tool)
	}
	if err := r.registry.RegisterBatch(tools); err != nil {
		return fmt.Errorf("%w: an extension tool name is already registered", extension.ErrToolNameConflict)
	}
	return nil
}

func (r *extensionToolRegistrar) UnregisterTools(_ context.Context, names []string) {
	r.registry.UnregisterBatch(names)
}

func (r *extensionToolRegistrar) runHTTP(ctx context.Context, declaration extension.ToolDeclaration, input json.RawMessage) (json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, declaration.Endpoint, bytes.NewReader(input))
	if err != nil {
		return nil, extension.ErrInvalidManifest
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return nil, errors.New("extension HTTP request failed")
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, extensionHTTPMaxResponse+1))
	if err != nil || len(data) > extensionHTTPMaxResponse {
		return nil, errors.New("extension HTTP response is invalid or exceeds its limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("extension HTTP endpoint returned status %d", response.StatusCode)
	}
	redacted := []byte(logging.RedactSensitiveText(string(data)))
	if !json.Valid(redacted) {
		return nil, errors.New("extension HTTP response is not valid JSON")
	}
	return json.RawMessage(redacted), nil
}
