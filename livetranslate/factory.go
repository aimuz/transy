// Package livetranslate provides real-time audio translation services
// using OpenAI Realtime API via WebRTC.
package livetranslate

import (
	"errors"

	"go.aimuz.me/transy/internal/types"
	"go.aimuz.me/transy/livetranslate/openai"
)

// Config holds configuration for creating a LiveTranslator.
// Zero values are replaced with sensible defaults.
type Config struct {
	APIKey       string
	Model        string // Default: "gpt-4o-realtime-preview"
	SystemPrompt string
	Temperature  float64 // Default: 0.6
}

// New creates a new LiveTranslator using OpenAI Realtime API.
func New(cfg Config) (types.LiveTranslator, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("livetranslate: API key required")
	}

	// Apply sensible defaults
	if cfg.Model == "" {
		cfg.Model = openai.DefaultModel
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.6
	}

	return openai.NewService(openai.ServiceConfig{
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		SystemPrompt: cfg.SystemPrompt,
		Temperature:  cfg.Temperature,
	})
}
