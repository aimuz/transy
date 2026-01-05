// Package types provides shared type definitions for the application.
package types

// Provider represents an LLM provider configuration.
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

// LiveTranscript represents a real-time transcription result.
type LiveTranscript struct {
	ID         string  `json:"id"`         // Unique identifier
	Text       string  `json:"text"`       // Original text
	Translated string  `json:"translated"` // Translated text
	Timestamp  int64   `json:"timestamp"`  // Unix timestamp in milliseconds
	IsFinal    bool    `json:"isFinal"`    // Whether this is the final result
	Confidence float64 `json:"confidence"` // Recognition confidence 0-1
}

// LiveStatus represents the status of live translation.
type LiveStatus struct {
	Active          bool   `json:"active"`
	SourceLang      string `json:"sourceLang"`
	TargetLang      string `json:"targetLang"`
	Duration        int64  `json:"duration"`        // Running duration in seconds
	STTProvider     string `json:"sttProvider"`     // Current STT provider name
	TranscriptCount int    `json:"transcriptCount"` // Number of transcribed segments
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
