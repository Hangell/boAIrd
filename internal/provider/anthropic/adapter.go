// Package anthropic provides the Anthropic Messages API adapter.
package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"boAIrd/internal/provider"
)

const DefaultBaseURL = "https://api.anthropic.com/v1"
const apiVersion = "2023-06-01"

type Adapter struct{ client *provider.JSONClient }

func New(config provider.Config) (*Adapter, error) {
	client, err := provider.NewJSONClient("anthropic", config, DefaultBaseURL, "x-api-key", "")
	if err != nil {
		return nil, err
	}
	return &Adapter{client: client}, nil
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatPayload struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
}
type messageResult struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (a *Adapter) Chat(ctx context.Context, request provider.ChatRequest) (provider.ChatResponse, error) {
	if err := provider.ValidateChatRequest(request); err != nil {
		return provider.ChatResponse{}, err
	}
	if request.Temperature != nil && *request.Temperature != 1 {
		return provider.ChatResponse{}, fmt.Errorf("anthropic: custom temperature is unsupported: %w", provider.ErrInvalidRequest)
	}
	payload := chatPayload{Model: request.Model, MaxTokens: request.MaxTokens}
	for _, item := range request.Messages {
		if item.Role == provider.RoleSystem {
			if payload.System != "" {
				payload.System += "\n\n"
			}
			payload.System += item.Content
			continue
		}
		payload.Messages = append(payload.Messages, message{Role: string(item.Role), Content: item.Content})
	}
	var result messageResult
	headers := http.Header{"Anthropic-Version": []string{apiVersion}}
	if err := a.client.Do(ctx, http.MethodPost, "messages", nil, headers, payload, &result); err != nil {
		return provider.ChatResponse{}, err
	}
	var content strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}
	if content.Len() == 0 {
		return provider.ChatResponse{}, fmt.Errorf("anthropic: %w", provider.ErrNoTextResponse)
	}
	return provider.ChatResponse{ID: result.ID, Model: result.Model, Content: content.String(), FinishReason: result.StopReason, Usage: provider.Usage{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens}}, nil
}

type modelPage struct {
	Data []struct {
		ID              string `json:"id"`
		DisplayName     string `json:"display_name"`
		ContextWindow   int    `json:"max_input_tokens"`
		MaxOutputTokens int    `json:"max_tokens"`
	} `json:"data"`
	HasMore bool   `json:"has_more"`
	LastID  string `json:"last_id"`
}

func (a *Adapter) Models(ctx context.Context) ([]provider.Model, error) {
	models := make([]provider.Model, 0)
	cursor := ""
	seen := make(map[string]struct{})
	for page := 0; page < 100; page++ {
		query := url.Values{"limit": []string{"100"}}
		if cursor != "" {
			query.Set("after_id", cursor)
		}
		var result modelPage
		if err := a.client.Do(ctx, http.MethodGet, "models", query, http.Header{"Anthropic-Version": []string{apiVersion}}, nil, &result); err != nil {
			return nil, err
		}
		for _, model := range result.Data {
			models = append(models, provider.Model{ID: model.ID, Name: model.DisplayName, ContextWindow: model.ContextWindow, MaxOutputTokens: model.MaxOutputTokens})
		}
		if !result.HasMore {
			return models, nil
		}
		if result.LastID == "" || result.LastID == cursor {
			return nil, fmt.Errorf("anthropic: %w", provider.ErrInvalidResponse)
		}
		if _, exists := seen[result.LastID]; exists {
			return nil, fmt.Errorf("anthropic: %w", provider.ErrInvalidResponse)
		}
		seen[result.LastID] = struct{}{}
		cursor = result.LastID
	}
	return nil, fmt.Errorf("anthropic: %w: too many model pages", provider.ErrInvalidResponse)
}

var _ provider.Provider = (*Adapter)(nil)
