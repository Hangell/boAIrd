// Package openai_compatible implements providers using the OpenAI chat API shape.
package openai_compatible

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"boAIrd/internal/provider"
)

const DefaultBaseURL = "https://api.openai.com/v1"

type Adapter struct {
	client     *provider.JSONClient
	name       string
	tokenField string
}

func New(config provider.Config) (*Adapter, error) {
	return NewNamed("openai-compatible", config, "", "max_tokens")
}

func NewNamed(name string, config provider.Config, defaultBaseURL, tokenField string) (*Adapter, error) {
	if tokenField != "max_tokens" && tokenField != "max_completion_tokens" {
		return nil, fmt.Errorf("%w: unsupported token field", provider.ErrInvalidConfiguration)
	}
	client, err := provider.NewJSONClient(name, config, defaultBaseURL, "Authorization", "Bearer ")
	if err != nil {
		return nil, err
	}
	return &Adapter{client: client, name: name, tokenField: tokenField}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatPayload struct {
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	MaxTokens           int           `json:"max_tokens,omitempty"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
	Temperature         *float64      `json:"temperature,omitempty"`
}

type chatResult struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (a *Adapter) Chat(ctx context.Context, request provider.ChatRequest) (provider.ChatResponse, error) {
	if err := provider.ValidateChatRequest(request); err != nil {
		return provider.ChatResponse{}, err
	}
	payload := chatPayload{Model: request.Model, Messages: make([]chatMessage, len(request.Messages)), Temperature: request.Temperature}
	if a.tokenField == "max_completion_tokens" {
		payload.MaxCompletionTokens = request.MaxTokens
	} else {
		payload.MaxTokens = request.MaxTokens
	}
	for i, message := range request.Messages {
		payload.Messages[i] = chatMessage{Role: string(message.Role), Content: message.Content}
	}
	var result chatResult
	if err := a.client.Do(ctx, http.MethodPost, "chat/completions", nil, nil, payload, &result); err != nil {
		return provider.ChatResponse{}, err
	}
	if len(result.Choices) == 0 {
		return provider.ChatResponse{}, fmt.Errorf("%s: %w", a.name, provider.ErrInvalidResponse)
	}
	if result.Choices[0].Message.Content == "" {
		return provider.ChatResponse{}, fmt.Errorf("%s: %w", a.name, provider.ErrNoTextResponse)
	}
	return provider.ChatResponse{ID: result.ID, Model: result.Model, Content: result.Choices[0].Message.Content, FinishReason: result.Choices[0].FinishReason, Usage: provider.Usage{InputTokens: result.Usage.PromptTokens, OutputTokens: result.Usage.CompletionTokens}}, nil
}

type modelList struct {
	Data []struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		OwnedBy          string   `json:"owned_by"`
		ContextWindow    int      `json:"context_window"`
		MaxOutputTokens  int      `json:"max_output_tokens"`
		InputModalities  []string `json:"input_modalities"`
		OutputModalities []string `json:"output_modalities"`
		Effort           struct {
			SupportedLevels []string `json:"supported_levels"`
			DefaultLevel    string   `json:"default_level"`
		} `json:"effort"`
	} `json:"data"`
}

func (a *Adapter) Models(ctx context.Context) ([]provider.Model, error) {
	var result modelList
	if err := a.client.Do(ctx, http.MethodGet, "models", url.Values{}, nil, nil, &result); err != nil {
		return nil, err
	}
	models := make([]provider.Model, 0, len(result.Data))
	for _, model := range result.Data {
		name := model.Name
		if name == "" {
			name = model.ID
		}
		models = append(models, provider.Model{
			ID: model.ID, Name: name, OwnedBy: model.OwnedBy,
			ContextWindow: model.ContextWindow, MaxOutputTokens: model.MaxOutputTokens,
			InputModalities: model.InputModalities, OutputModalities: model.OutputModalities,
			SupportedEffortLevels: model.Effort.SupportedLevels, DefaultEffortLevel: model.Effort.DefaultLevel,
		})
	}
	return models, nil
}
