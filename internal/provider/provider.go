// Package provider defines the provider-independent chat and model contract.
package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

var (
	ErrInvalidRequest        = errors.New("invalid provider request")
	ErrInvalidConfiguration  = errors.New("invalid provider configuration")
	ErrCredentialUnavailable = errors.New("provider credential unavailable")
	ErrProviderNotFound      = errors.New("provider not found")
	ErrProviderAlreadyExists = errors.New("provider already registered")
	ErrInvalidProvider       = errors.New("provider is required")
	ErrInvalidResponse       = errors.New("invalid provider response")
	ErrNoTextResponse        = errors.New("provider response contains no text")
)

// Provider generates chat responses and discovers models available to its credential.
type Provider interface {
	Chat(context.Context, ChatRequest) (ChatResponse, error)
	Models(context.Context) ([]Model, error)
}

type ChatMessage struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature *float64      `json:"temperature,omitempty"`
}

type ChatResponse struct {
	ID           string `json:"id,omitempty"`
	Model        string `json:"model"`
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason,omitempty"`
	Usage        Usage  `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Model struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name,omitempty"`
	OwnedBy               string   `json:"owned_by,omitempty"`
	ContextWindow         int      `json:"context_window,omitempty"`
	MaxOutputTokens       int      `json:"max_output_tokens,omitempty"`
	InputModalities       []string `json:"input_modalities,omitempty"`
	OutputModalities      []string `json:"output_modalities,omitempty"`
	SupportedEffortLevels []string `json:"supported_effort_levels,omitempty"`
	DefaultEffortLevel    string   `json:"default_effort_level,omitempty"`
}

// Registry resolves named provider instances configured by the application.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	factories map[string]func(Config) (Provider, error)
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider), factories: make(map[string]func(Config) (Provider, error))}
}

func (r *Registry) RegisterFactory(providerType string, factory func(Config) (Provider, error)) error {
	providerType = strings.TrimSpace(providerType)
	if providerType == "" || factory == nil {
		return fmt.Errorf("%w: provider type and factory are required", ErrInvalidConfiguration)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[providerType]; exists {
		return fmt.Errorf("%w: factory %s", ErrProviderAlreadyExists, providerType)
	}
	r.factories[providerType] = factory
	return nil
}

func (r *Registry) Configure(id, providerType string, config Config) error {
	r.mu.RLock()
	factory, exists := r.factories[strings.TrimSpace(providerType)]
	r.mu.RUnlock()
	if !exists {
		return fmt.Errorf("%w: provider type %s", ErrProviderNotFound, providerType)
	}
	configured, err := factory(config)
	if err != nil {
		return err
	}
	return r.Register(id, configured)
}

func (r *Registry) Register(id string, provider Provider) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: provider ID is required", ErrInvalidConfiguration)
	}
	if provider == nil {
		return ErrInvalidProvider
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("%w: %s", ErrProviderAlreadyExists, id)
	}
	r.providers[id] = provider
	return nil
}

func (r *Registry) Resolve(id string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, exists := r.providers[strings.TrimSpace(id)]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, id)
	}
	return provider, nil
}

func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// CredentialResolver retrieves a secret by reference without storing it in provider configuration.
type CredentialResolver interface {
	Resolve(context.Context, string) (string, error)
}

// Config identifies the API endpoint and credential reference used by an adapter.
type Config struct {
	BaseURL            string
	CredentialID       string
	CredentialResolver CredentialResolver
	HTTPClient         *http.Client
}

func ValidateChatRequest(request ChatRequest) error {
	if strings.TrimSpace(request.Model) == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidRequest)
	}
	if len(request.Messages) == 0 {
		return fmt.Errorf("%w: at least one message is required", ErrInvalidRequest)
	}
	if request.MaxTokens < 1 {
		return fmt.Errorf("%w: max_tokens must be positive", ErrInvalidRequest)
	}
	if request.Temperature != nil && (*request.Temperature < 0 || *request.Temperature > 2) {
		return fmt.Errorf("%w: temperature must be between 0 and 2", ErrInvalidRequest)
	}
	seenConversationMessage := false
	for index, message := range request.Messages {
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("%w: message %d content is required", ErrInvalidRequest, index)
		}
		switch message.Role {
		case RoleSystem:
			if seenConversationMessage {
				return fmt.Errorf("%w: system messages must precede conversation messages", ErrInvalidRequest)
			}
		case RoleUser, RoleAssistant:
			seenConversationMessage = true
		default:
			return fmt.Errorf("%w: unsupported message role %q", ErrInvalidRequest, message.Role)
		}
	}
	if !seenConversationMessage {
		return fmt.Errorf("%w: at least one user or assistant message is required", ErrInvalidRequest)
	}
	return nil
}
