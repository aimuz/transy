// Package livetranslate provides real-time audio translation services.
// It supports multiple backends: local (VAD + STT + LLM translation) and
// cloud-based (e.g., OpenAI Realtime API).
package livetranslate

import (
	"context"
	"fmt"

	"go.aimuz.me/transy/internal/types"
	"go.aimuz.me/transy/livetranslate/realtime"
	"go.aimuz.me/transy/stt"
)

// FactoryConfig holds configuration for creating a LiveTranslator.
// The Mode field determines which implementation is used.
type FactoryConfig struct {
	// Mode selects the implementation: "realtime" or "transcription" (default)
	Mode string

	// Realtime mode configuration (OpenAI Realtime API via WebRTC)
	APIKey       string
	Model        string
	SystemPrompt string
	Temperature  float64

	// Transcription mode configuration (VAD + STT + LLM)
	STTProvider   stt.Provider
	TranslateFunc TranslateFunc
	VADThreshold  float32
}

// New creates a new LiveTranslator based on the configuration.
// It internally selects the appropriate implementation.
func New(cfg FactoryConfig) (types.LiveTranslator, error) {
	if cfg.Mode == "realtime" {
		return newRealtimeTranslator(cfg)
	}
	return newLocalTranslator(cfg)
}

// newRealtimeTranslator creates an OpenAI Realtime API based translator (WebRTC).
func newRealtimeTranslator(cfg FactoryConfig) (types.LiveTranslator, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key required for realtime mode")
	}

	srv, err := realtime.NewService(realtime.ServiceConfig{
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		SystemPrompt: cfg.SystemPrompt,
		Temperature:  cfg.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("create realtime service: %w", err)
	}

	return srv, nil
}

// newLocalTranslator creates a local VAD + STT + LLM translation pipeline.
func newLocalTranslator(cfg FactoryConfig) (types.LiveTranslator, error) {
	if cfg.STTProvider == nil {
		return nil, fmt.Errorf("STT provider required for transcription mode")
	}

	localCfg := DefaultConfig()
	localCfg.STTProvider = cfg.STTProvider
	localCfg.TranslateFunc = cfg.TranslateFunc

	if cfg.VADThreshold > 0 {
		localCfg.VADThreshold = cfg.VADThreshold
	}

	srv, err := NewService(localCfg)
	if err != nil {
		return nil, fmt.Errorf("create local service: %w", err)
	}

	return srv, nil
}

// NewOrReuse returns the existing translator if compatible, or creates a new one.
// This handles mode switching automatically.
func NewOrReuse(current types.LiveTranslator, cfg FactoryConfig) (types.LiveTranslator, error) {
	if current != nil {
		_, isRealtime := current.(*realtime.Service)
		wantRealtime := cfg.Mode == "realtime"

		// Same mode: reuse existing
		if isRealtime == wantRealtime {
			return current, nil
		}

		// Mode changed: stop old, create new
		_ = current.Stop() // Ignore error, just try to stop
	}

	return New(cfg)
}

// StartSession is a convenience wrapper that creates (or reuses) a translator and starts it.
func StartSession(current types.LiveTranslator, cfg FactoryConfig, ctx context.Context, sourceLang, targetLang string) (types.LiveTranslator, error) {
	translator, err := NewOrReuse(current, cfg)
	if err != nil {
		return nil, err
	}

	if err := translator.Start(ctx, sourceLang, targetLang); err != nil {
		return nil, fmt.Errorf("start translator: %w", err)
	}

	return translator, nil
}
