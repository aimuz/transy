package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.aimuz.me/transy/internal/types"
)

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// geminiCompleter implements Completer for Gemini API.
type geminiCompleter struct {
	cfg completerConfig
}

// Gemini request/response types
type geminiRequest struct {
	Contents          []geminiContent   `json:"contents"`
	GenerationConfig  geminiConfig      `json:"generationConfig,omitempty"`
	SystemInstruction *geminiSystemInst `json:"systemInstruction,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	Temperature     float64         `json:"temperature,omitempty"`
	ThinkingConfig  *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type thinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget"`
}

type geminiSystemInst struct {
	Parts []geminiPart `json:"parts"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
	Error         *geminiError      `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *geminiCompleter) Complete(ctx context.Context, messages []Message) (string, types.Usage, error) {
	// Convert messages to Gemini format
	var parts []geminiContent
	var systemPrompt string

	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt += msg.Content + "\n"
			continue
		}

		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}

		parts = append(parts, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	reqBody := geminiRequest{
		Contents: parts,
		GenerationConfig: geminiConfig{
			MaxOutputTokens: c.cfg.maxTokens,
			Temperature:     c.cfg.temperature,
		},
	}

	// Disable thinking for Gemini 2.5 Flash models if requested
	if c.cfg.disableThinking {
		reqBody.GenerationConfig.ThinkingConfig = &thinkingConfig{
			ThinkingBudget: 0,
		}
	}

	if systemPrompt != "" {
		reqBody.SystemInstruction = &geminiSystemInst{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("marshal request: %w", err)
	}

	baseURL := defaultGeminiBaseURL
	if c.cfg.baseURL != "" {
		baseURL = c.cfg.baseURL
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", baseURL, c.cfg.model, c.cfg.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.cfg.http.Do(req)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("read response: %w", err)
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", types.Usage{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if geminiResp.Error != nil {
		return "", types.Usage{}, fmt.Errorf("api error: %d - %s", geminiResp.Error.Code, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", types.Usage{}, fmt.Errorf("no candidates returned")
	}

	var usage types.Usage
	if geminiResp.UsageMetadata != nil {
		usage = types.Usage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		}
	}

	return geminiResp.Candidates[0].Content.Parts[0].Text, usage, nil
}
