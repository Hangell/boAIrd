// Package gemini provides the Google Gemini generateContent adapter.
package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"boAIrd/internal/provider"
)

const DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

type Adapter struct{ client *provider.JSONClient }

func New(config provider.Config) (*Adapter, error) {
	client, err := provider.NewJSONClient("gemini", config, DefaultBaseURL, "x-goog-api-key", "")
	if err != nil {
		return nil, err
	}
	return &Adapter{client: client}, nil
}

type part struct {
	Text string `json:"text"`
}
type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}
type chatPayload struct {
	SystemInstruction *content  `json:"systemInstruction,omitempty"`
	Contents          []content `json:"contents"`
	GenerationConfig  struct {
		MaxOutputTokens int      `json:"maxOutputTokens"`
		Temperature     *float64 `json:"temperature,omitempty"`
	} `json:"generationConfig"`
}
type chatResult struct {
	Candidates []struct {
		Content      content `json:"content"`
		FinishReason string  `json:"finishReason"`
	} `json:"candidates"`
	Usage struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func (a *Adapter) Chat(ctx context.Context, request provider.ChatRequest) (provider.ChatResponse, error) {
	if err := provider.ValidateChatRequest(request); err != nil {
		return provider.ChatResponse{}, err
	}
	payload := chatPayload{}
	payload.GenerationConfig.MaxOutputTokens = request.MaxTokens
	payload.GenerationConfig.Temperature = request.Temperature
	for _, item := range request.Messages {
		entry := content{Parts: []part{{Text: item.Content}}}
		switch item.Role {
		case provider.RoleSystem:
			if payload.SystemInstruction == nil {
				payload.SystemInstruction = &content{}
			}
			payload.SystemInstruction.Parts = append(payload.SystemInstruction.Parts, entry.Parts...)
		case provider.RoleAssistant:
			entry.Role = "model"
			payload.Contents = append(payload.Contents, entry)
		case provider.RoleUser:
			entry.Role = "user"
			payload.Contents = append(payload.Contents, entry)
		}
	}
	var result chatResult
	resource := "models/" + strings.TrimPrefix(request.Model, "models/") + ":generateContent"
	if err := a.client.Do(ctx, http.MethodPost, resource, nil, nil, payload, &result); err != nil {
		return provider.ChatResponse{}, err
	}
	if len(result.Candidates) == 0 {
		return provider.ChatResponse{}, fmt.Errorf("gemini: %w", provider.ErrInvalidResponse)
	}
	var text strings.Builder
	for _, candidatePart := range result.Candidates[0].Content.Parts {
		text.WriteString(candidatePart.Text)
	}
	if text.Len() == 0 {
		return provider.ChatResponse{}, fmt.Errorf("gemini: %w", provider.ErrNoTextResponse)
	}
	return provider.ChatResponse{Model: request.Model, Content: text.String(), FinishReason: result.Candidates[0].FinishReason, Usage: provider.Usage{InputTokens: result.Usage.PromptTokenCount, OutputTokens: result.Usage.CandidatesTokenCount}}, nil
}

type modelPage struct {
	Models []struct {
		Name                       string   `json:"name"`
		DisplayName                string   `json:"displayName"`
		InputTokenLimit            int      `json:"inputTokenLimit"`
		OutputTokenLimit           int      `json:"outputTokenLimit"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
	NextPageToken string `json:"nextPageToken"`
}

func (a *Adapter) Models(ctx context.Context) ([]provider.Model, error) {
	models := make([]provider.Model, 0)
	pageToken := ""
	seen := make(map[string]struct{})
	for page := 0; page < 100; page++ {
		query := url.Values{"pageSize": []string{"1000"}}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		var result modelPage
		if err := a.client.Do(ctx, http.MethodGet, "models", query, nil, nil, &result); err != nil {
			return nil, err
		}
		for _, model := range result.Models {
			if !supportsGenerate(model.SupportedGenerationMethods) {
				continue
			}
			id := strings.TrimPrefix(model.Name, "models/")
			models = append(models, provider.Model{ID: id, Name: model.DisplayName, ContextWindow: model.InputTokenLimit, MaxOutputTokens: model.OutputTokenLimit})
		}
		if result.NextPageToken == "" {
			return models, nil
		}
		if _, exists := seen[result.NextPageToken]; exists {
			return nil, fmt.Errorf("gemini: %w", provider.ErrInvalidResponse)
		}
		seen[result.NextPageToken] = struct{}{}
		pageToken = result.NextPageToken
	}
	return nil, fmt.Errorf("gemini: %w: too many model pages", provider.ErrInvalidResponse)
}
func supportsGenerate(methods []string) bool {
	for _, method := range methods {
		if method == "generateContent" {
			return true
		}
	}
	return false
}

var _ provider.Provider = (*Adapter)(nil)
