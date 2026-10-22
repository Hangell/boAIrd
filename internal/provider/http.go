package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"boAIrd/internal/credential"
)

const (
	defaultRequestTimeout = 90 * time.Second
	maximumResponseBytes  = 8 << 20
	maximumRequestBytes   = 4 << 20
)

// HTTPError reports a provider HTTP status without exposing its response body.
type HTTPError struct {
	Provider   string
	Operation  string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s returned HTTP %d", e.Provider, e.Operation, e.StatusCode)
}

type JSONClient struct {
	name            string
	baseURL         string
	credentialID    string
	credentials     CredentialResolver
	credentialScope credential.Scope
	authHeader      string
	authPrefix      string
	httpClient      *http.Client
}

func NewJSONClient(name string, config Config, defaultBaseURL, authHeader, authPrefix string) (*JSONClient, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(authHeader) == "" {
		return nil, fmt.Errorf("%w: provider name and credential header are required", ErrInvalidConfiguration)
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsedURL, err := validateBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfiguration, err)
	}
	if strings.TrimSpace(config.CredentialID) == "" {
		return nil, fmt.Errorf("%w: credential ID is required", ErrInvalidConfiguration)
	}
	if config.CredentialResolver == nil {
		return nil, fmt.Errorf("%w: credential resolver is required", ErrInvalidConfiguration)
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultRequestTimeout}
	} else {
		clientCopy := *client
		client = &clientCopy
		if client.Timeout <= 0 {
			client.Timeout = defaultRequestTimeout
		}
	}
	// Provider credentials must never follow a redirect to another origin.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &JSONClient{
		name:            name,
		baseURL:         strings.TrimRight(parsedURL.String(), "/"),
		credentialID:    config.CredentialID,
		credentials:     config.CredentialResolver,
		credentialScope: config.CredentialScope,
		authHeader:      authHeader,
		authPrefix:      authPrefix,
		httpClient:      client,
	}, nil
}

func (c *JSONClient) Do(ctx context.Context, method, resourcePath string, query url.Values, headers http.Header, requestBody, responseBody any) error {
	if ctx == nil {
		return fmt.Errorf("%s request: context is required", c.name)
	}
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("%s encode request: %w", c.name, err)
		}
		if len(encoded) > maximumRequestBytes {
			return fmt.Errorf("%s request exceeds %d bytes", c.name, maximumRequestBytes)
		}
		body = bytes.NewReader(encoded)
	}
	endpoint, err := c.endpointURL(resourcePath, query)
	if err != nil {
		return fmt.Errorf("%s build endpoint: %w", c.name, err)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("%s create request: %w", c.name, err)
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	credential, err := c.credentials.Resolve(ctx, c.credentialID, c.credentialScope)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%s resolve credential: %w", c.name, err)
		}
		return fmt.Errorf("%s: %w", c.name, ErrCredentialUnavailable)
	}
	if strings.TrimSpace(credential) == "" {
		return fmt.Errorf("%s: %w", c.name, ErrCredentialUnavailable)
	}
	request.Header.Set(c.authHeader, c.authPrefix+credential)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", c.name, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maximumResponseBytes))
		return &HTTPError{Provider: c.name, Operation: method + " " + resourcePath, StatusCode: response.StatusCode}
	}
	if responseBody == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maximumResponseBytes))
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%s read response: %w", c.name, err)
	}
	if len(data) > maximumResponseBytes {
		return fmt.Errorf("%s: %w: response exceeds %d bytes", c.name, ErrInvalidResponse, maximumResponseBytes)
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("%s: %w", c.name, ErrInvalidResponse)
	}
	return nil
}

func (c *JSONClient) endpointURL(resourcePath string, query url.Values) (string, error) {
	if strings.Contains(resourcePath, "?") || strings.Contains(resourcePath, "#") {
		return "", errors.New("resource path must not contain a query or fragment")
	}
	if strings.HasPrefix(resourcePath, "/") {
		return "", errors.New("resource path must be relative")
	}
	for _, segment := range strings.Split(resourcePath, "/") {
		if segment == ".." || segment == "." {
			return "", errors.New("resource path must not traverse directories")
		}
	}
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(baseURL.Path, "/")
	if basePath == "" {
		basePath = "/"
	}
	if basePath != "/" {
		baseURL.Path = basePath + "/" + resourcePath
	} else {
		baseURL.Path = "/" + resourcePath
	}
	baseURL.RawPath = ""
	baseURL.RawQuery = query.Encode()
	return baseURL.String(), nil
}

func validateBaseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("base URL must include a host and must not contain credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("base URL must use HTTPS; HTTP is allowed only for loopback endpoints")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		address := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (address == nil || !address.IsLoopback()) {
			return nil, errors.New("HTTP base URL must target loopback")
		}
	}
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return nil, errors.New("base URL path is invalid")
	}
	for _, segment := range strings.Split(decodedPath, "/") {
		if segment == ".." || segment == "." {
			return nil, errors.New("base URL path must not traverse parent directories")
		}
	}
	return parsed, nil
}
