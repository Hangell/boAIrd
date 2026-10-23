package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"boAIrd/internal/extension"
	"boAIrd/internal/logging"
	governedtool "boAIrd/internal/tool"
)

const (
	extensionHTTPTimeout     = 30 * time.Second
	extensionHTTPMaxResponse = 256 << 10
)

// extensionToolRegistrar adapts validated extension tool declarations to the
// governed tool registry. Declarative tools are contract-only and are not
// executable; http tools perform one bounded, redacted request to their
// administrator-approved HTTPS endpoint and can never touch the filesystem,
// secrets, or an arbitrary program.
type extensionToolRegistrar struct {
	registry *governedtool.Registry
	client   *http.Client
}

func newExtensionToolRegistrar(registry *governedtool.Registry) *extensionToolRegistrar {
	return &extensionToolRegistrar{registry: registry, client: &http.Client{Timeout: extensionHTTPTimeout}}
}

func (r *extensionToolRegistrar) RegisterTool(_ context.Context, declaration extension.ToolDeclaration) error {
	if declaration.Kind == extension.KindDeclarative {
		return nil
	}
	if declaration.Kind != extension.KindHTTP {
		return fmt.Errorf("%w: unsupported extension tool kind %q", extension.ErrInvalidManifest, declaration.Kind)
	}
	tool := governedtool.Definition[json.RawMessage, json.RawMessage]{
		Metadata: governedtool.Contract{
			Name:                 declaration.Name,
			Description:          declaration.Description,
			InputSchema:          declaration.InputSchema,
			OutputSchema:         declaration.OutputSchema,
			RequiredCapabilities: declaration.RequiredCapabilities,
			Limits:               governedtool.Limits{Timeout: extensionHTTPTimeout, MaxInputBytes: 64 << 10, MaxOutputBytes: extensionHTTPMaxResponse + 4096},
		},
		Validate:       func(json.RawMessage) error { return nil },
		ValidateOutput: func(json.RawMessage) error { return nil },
		RunWithRequest: func(ctx context.Context, _ governedtool.Request, input json.RawMessage) (json.RawMessage, error) {
			return r.runHTTP(ctx, declaration, input)
		},
	}
	return r.registry.Register(tool)
}

func (r *extensionToolRegistrar) UnregisterTool(_ context.Context, name string) {
	r.registry.Unregister(name)
}

func (r *extensionToolRegistrar) runHTTP(ctx context.Context, declaration extension.ToolDeclaration, input json.RawMessage) (json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, declaration.Endpoint, bytes.NewReader(input))
	if err != nil {
		return nil, extension.ErrInvalidManifest
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("extension HTTP request failed")
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, extensionHTTPMaxResponse))
	if err != nil {
		return nil, fmt.Errorf("extension HTTP response failed")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("extension HTTP endpoint returned status %d", response.StatusCode)
	}
	return json.RawMessage(logging.RedactSensitiveText(string(data))), nil
}
