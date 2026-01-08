// Package llm provides HTTP clients for LLM API calls.
package llm

import (
	"context"
	"net/http"

	"go.aimuz.me/transy/internal/types"
)

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Options configures LLM completion behavior.
type Options struct {
	MaxTokens       int
	Temperature     float64
	DisableThinking bool // For Gemini: set thinkingBudget to 0
}

// Completer performs chat completions.
type Completer interface {
	Complete(ctx context.Context, messages []Message) (string, types.Usage, error)
}

// completerConfig holds all parameters needed by completers.
// Memory layout optimized: pointers/slices first, then 64-bit, then smaller.
type completerConfig struct {
	http            *http.Client
	apiKey          string
	baseURL         string
	model           string
	maxTokens       int
	temperature     float64
	disableThinking bool
}

// NewCompleter creates a Completer for the given provider type.
func NewCompleter(apiType, apiKey, baseURL, model string, opts Options) Completer {
	cfg := completerConfig{
		http:            &http.Client{},
		apiKey:          apiKey,
		baseURL:         baseURL,
		model:           model,
		maxTokens:       opts.MaxTokens,
		temperature:     opts.Temperature,
		disableThinking: opts.DisableThinking,
	}

	switch apiType {
	case "gemini":
		return &geminiCompleter{cfg: cfg}
	case "claude":
		return &claudeCompleter{cfg: cfg}
	case "openai", "openai-compatible":
		return &openaiCompleter{cfg: cfg, isCompatible: apiType == "openai-compatible"}
	default:
		// Default to OpenAI format
		return &openaiCompleter{cfg: cfg}
	}
}
