// Package adapters wires the supported provider implementations into a registry.
package adapters

import (
	"boAIrd/internal/provider"
	"boAIrd/internal/provider/anthropic"
	"boAIrd/internal/provider/deepseek"
	"boAIrd/internal/provider/gemini"
	"boAIrd/internal/provider/openai"
	"boAIrd/internal/provider/openai_compatible"
)

// Register installs factories for the built-in providers. OpenAI-compatible
// providers require an explicit BaseURL to avoid silently targeting a vendor.
func Register(registry *provider.Registry) error {
	factories := []struct {
		name string
		new  func(provider.Config) (provider.Provider, error)
	}{
		{"openai", func(config provider.Config) (provider.Provider, error) { return openai.New(config) }},
		{"anthropic", func(config provider.Config) (provider.Provider, error) { return anthropic.New(config) }},
		{"deepseek", func(config provider.Config) (provider.Provider, error) { return deepseek.New(config) }},
		{"gemini", func(config provider.Config) (provider.Provider, error) { return gemini.New(config) }},
		{"openai-compatible", func(config provider.Config) (provider.Provider, error) { return openai_compatible.New(config) }},
	}
	for _, item := range factories {
		if err := registry.RegisterFactory(item.name, item.new); err != nil {
			return err
		}
	}
	return nil
}
