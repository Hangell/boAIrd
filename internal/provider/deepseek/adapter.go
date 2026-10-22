// Package deepseek provides the DeepSeek chat completions adapter.
package deepseek

import (
	"boAIrd/internal/provider"
	compatible "boAIrd/internal/provider/openai_compatible"
)

type Adapter struct{ *compatible.Adapter }

func New(config provider.Config) (*Adapter, error) {
	adapter, err := compatible.NewNamed("deepseek", config, "https://api.deepseek.com", "max_tokens")
	if err != nil {
		return nil, err
	}
	return &Adapter{Adapter: adapter}, nil
}

var _ provider.Provider = (*Adapter)(nil)
