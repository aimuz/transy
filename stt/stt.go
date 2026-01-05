// Package stt provides speech-to-text provider interface and implementations.
package stt

import "time"

// TranscribeResult represents the result of a transcription.
type TranscribeResult struct {
	Text       string    `json:"text"`       // Transcribed text
	Language   string    `json:"language"`   // Detected language code
	Confidence float64   `json:"confidence"` // Recognition confidence 0-1
	Segments   []Segment `json:"segments"`   // Time-stamped segments
}

// Segment represents a time-stamped audio segment.
type Segment struct {
	Text  string        `json:"text"`
	Start time.Duration `json:"start"`
	End   time.Duration `json:"end"`
}

// Provider defines the interface for speech-to-text providers.
// Both local (whisper.cpp) and remote (OpenAI API) implementations
// must satisfy this interface.
type Provider interface {
	// Name returns the provider identifier.
	Name() string

	// DisplayName returns the human-readable provider name.
	DisplayName() string

	// IsLocal returns true if the provider runs locally without API calls.
	IsLocal() bool

	// RequiresSetup returns true if setup is needed (e.g., model download).
	RequiresSetup() bool

	// IsReady returns true if the provider is ready to use.
	IsReady() bool

	// SetupProgress returns the setup progress (0-100), -1 if not started.
	SetupProgress() int

	// Setup performs initialization (e.g., download model).
	// The progress callback receives percentage (0-100).
	Setup(progress func(percent int)) error

	// Transcribe converts audio samples to text.
	// audio: PCM float32 samples at 16000 Hz sample rate
	// language: source language code (empty for auto-detect)
	Transcribe(audio []float32, language string) (*TranscribeResult, error)

	// Close releases resources held by the provider.
	Close() error
}

// Registry holds registered STT providers.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates a new provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Get returns a provider by name.
func (r *Registry) Get(name string) Provider {
	return r.providers[name]
}

// List returns all registered providers.
func (r *Registry) List() []Provider {
	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// Close releases all providers.
func (r *Registry) Close() error {
	for _, p := range r.providers {
		if err := p.Close(); err != nil {
			return err
		}
	}
	return nil
}
