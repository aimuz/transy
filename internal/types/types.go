// Package types provides shared type definitions for the application.
package types

import "context"

// Provider represents an LLM provider configuration.
// Deprecated: Use APICredential + TranslationProfile instead.
type Provider struct {
	Name            string  `json:"name"`
	Type            string  `json:"type"` // "openai", "openai-compatible", "gemini", "claude"
	BaseURL         string  `json:"base_url,omitempty"`
	APIKey          string  `json:"api_key"`
	Model           string  `json:"model"`
	SystemPrompt    string  `json:"system_prompt,omitempty"`
	MaxTokens       int     `json:"max_tokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	Active          bool    `json:"active"`
	DisableThinking bool    `json:"disable_thinking,omitempty"` // For Gemini: set thinkingBudget to 0
}

// ─────────────────────────────────────────────────────────────────────────────
// New Configuration Architecture
// ─────────────────────────────────────────────────────────────────────────────

// APICredential represents a reusable API credential.
// One credential can be used by multiple translation profiles or speech services.
type APICredential struct {
	ID      string `json:"id"`                 // UUID for reference
	Name    string `json:"name"`               // Display name, e.g., "My OpenAI"
	Type    string `json:"type"`               // "openai", "openai-compatible", "gemini", "claude"
	BaseURL string `json:"base_url,omitempty"` // Custom endpoint (required for openai-compatible)
	APIKey  string `json:"api_key"`
}

// TranslationProfile represents a translation configuration bound to an API credential.
type TranslationProfile struct {
	ID              string  `json:"id"`            // UUID
	Name            string  `json:"name"`          // Display name
	CredentialID    string  `json:"credential_id"` // Reference to APICredential.ID
	Model           string  `json:"model"`         // Model to use
	SystemPrompt    string  `json:"system_prompt,omitempty"`
	MaxTokens       int     `json:"max_tokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
	Active          bool    `json:"active"` // Currently active profile
	DisableThinking bool    `json:"disable_thinking,omitempty"`
}

// SpeechConfig represents speech service configuration (STT, speech translation, etc).
// Requires an OpenAI-compatible API credential.
type SpeechConfig struct {
	Enabled      bool   `json:"enabled"`       // Whether speech API is enabled
	CredentialID string `json:"credential_id"` // Reference to APICredential.ID
	Model        string `json:"model"`         // e.g., "whisper-1" or "gpt-4o-realtime-preview"
	Mode         string `json:"mode"`          // "transcription" (default) or "realtime"
}

// DefaultMaxTokens is the default max tokens if not specified.
const DefaultMaxTokens = 1000

// DefaultTemperature is the default temperature if not specified.
const DefaultTemperature = 0.3

// TranslateRequest represents a translation request from the frontend.
type TranslateRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
	Context    string `json:"context,omitempty"` // Previous context for better coherence
}

// DetectResult represents the result of language detection.
type DetectResult struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	DefaultTarget string `json:"defaultTarget"`
}

// Usage represents token usage statistics from LLM API calls.
type Usage struct {
	PromptTokens     int  `json:"promptTokens"`
	CompletionTokens int  `json:"completionTokens"`
	TotalTokens      int  `json:"totalTokens"`
	CacheHit         bool `json:"cacheHit"`
}

// TranslateResult represents the result of a translation request.
type TranslateResult struct {
	Text  string `json:"text"`
	Usage Usage  `json:"usage"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Live Translation Types
// ─────────────────────────────────────────────────────────────────────────────

// LiveTranscript represents a real-time transcription result with bilingual support.
type LiveTranscript struct {
	ID         string  `json:"id"`         // Unique identifier
	SourceText string  `json:"sourceText"` // Original transcribed text
	TargetText string  `json:"targetText"` // Translated text (may be empty if pending)
	SourceLang string  `json:"sourceLang"` // Source language code
	TargetLang string  `json:"targetLang"` // Target language code
	StartTime  int64   `json:"startTime"`  // Segment start time (ms since session start)
	EndTime    int64   `json:"endTime"`    // Segment end time (ms since session start, 0 if ongoing)
	Timestamp  int64   `json:"timestamp"`  // Unix timestamp in milliseconds (creation time)
	IsFinal    bool    `json:"isFinal"`    // Whether this is the final result
	Confidence float64 `json:"confidence"` // Recognition confidence 0-1
}

// VADState represents the current voice activity state.
type VADState string

const (
	VADStateListening  VADState = "listening"
	VADStateSpeaking   VADState = "speaking"
	VADStateProcessing VADState = "processing"
)

// LiveStatus represents the status of live translation.
type LiveStatus struct {
	Active          bool     `json:"active"`
	SourceLang      string   `json:"sourceLang"`
	TargetLang      string   `json:"targetLang"`
	Duration        int64    `json:"duration"`        // Running duration in seconds
	STTProvider     string   `json:"sttProvider"`     // Current STT provider name
	TranscriptCount int      `json:"transcriptCount"` // Number of transcribed segments
	VADState        VADState `json:"vadState"`        // Current VAD state
}

// STTProviderInfo represents information about an STT provider.
type STTProviderInfo struct {
	Name          string `json:"name"`          // Provider identifier
	DisplayName   string `json:"displayName"`   // Human-readable name
	IsLocal       bool   `json:"isLocal"`       // Whether it runs locally
	RequiresSetup bool   `json:"requiresSetup"` // Whether setup is needed (e.g., model download)
	SetupProgress int    `json:"setupProgress"` // Setup progress 0-100, -1 if not started
	IsReady       bool   `json:"isReady"`       // Whether the provider is ready to use
}

// LiveTranslator provides real-time speech translation with a unified interface.
// This interface abstracts both local (VAD+STT+Translation) and API-based (e.g., OpenAI Realtime)
// implementations, making them interchangeable.
type LiveTranslator interface {
	// Start begins live translation.
	// ctx allows cancellation and deadline control.
	Start(ctx context.Context, sourceLang, targetLang string) error

	// Stop stops the live translation service.
	Stop() error

	// Transcripts returns a read-only channel for receiving transcripts.
	// The channel is closed when the service stops.
	Transcripts() <-chan LiveTranscript

	// Errors returns a read-only channel for receiving errors.
	// The channel is closed when the service stops.
	Errors() <-chan error

	// Status returns the current status of the translation service.
	Status() LiveStatus

	// VADUpdates returns a channel for receiving Voice Activity Detection state changes.
	VADUpdates() <-chan VADState
}
