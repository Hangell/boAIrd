// Package openai provides the OpenAI chat completions adapter.
package openai

import (
	"boAIrd/internal/provider"
	compatible "boAIrd/internal/provider/openai_compatible"
)

type Adapter struct{ *compatible.Adapter }

func New(config provider.Config) (*Adapter, error) {
	adapter, err := compatible.NewNamed("openai", config, "https://api.openai.com/v1", "max_completion_tokens")
	if err != nil {
		return nil, err
	}
	return &Adapter{Adapter: adapter}, nil
}

var _ provider.Provider = (*Adapter)(nil)
