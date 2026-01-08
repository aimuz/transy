package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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

// buildRequest constructs the Gemini API request body from messages.
func (c *geminiCompleter) buildRequest(messages []Message) geminiRequest {
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

	req := geminiRequest{
		Contents: parts,
		GenerationConfig: geminiConfig{
			MaxOutputTokens: c.cfg.maxTokens,
			Temperature:     c.cfg.temperature,
		},
	}

	if c.cfg.disableThinking {
		req.GenerationConfig.ThinkingConfig = &thinkingConfig{
			ThinkingBudget: 0,
		}
	}

	if systemPrompt != "" {
		req.SystemInstruction = &geminiSystemInst{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	return req
}

// baseURL returns the configured or default base URL.
func (c *geminiCompleter) baseURL() string {
	if c.cfg.baseURL != "" {
		return c.cfg.baseURL
	}
	return defaultGeminiBaseURL
}

func (c *geminiCompleter) Complete(ctx context.Context, messages []Message) (string, types.Usage, error) {
	reqBody := c.buildRequest(messages)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", c.baseURL(), c.cfg.model, c.cfg.apiKey)

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

	return geminiResp.Candidates[0].Content.Parts[0].Text, geminiToUsage(geminiResp.UsageMetadata), nil
}

// StreamComplete implements StreamCompleter for streaming responses.
func (c *geminiCompleter) StreamComplete(ctx context.Context, messages []Message) (<-chan StreamDelta, error) {
	reqBody := c.buildRequest(messages)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse&key=%s", c.baseURL(), c.cfg.model, c.cfg.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.cfg.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("api error: %d - %s", resp.StatusCode, string(body))
	}

	ch := make(chan StreamDelta, 16)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		var usage types.Usage
		scanner := bufio.NewScanner(resp.Body)

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			var chunk geminiResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
				if text := chunk.Candidates[0].Content.Parts[0].Text; text != "" {
					select {
					case ch <- StreamDelta{Text: text}:
					case <-ctx.Done():
						return
					}
				}
			}

			if chunk.UsageMetadata != nil {
				usage = geminiToUsage(chunk.UsageMetadata)
			}
		}

		select {
		case ch <- StreamDelta{Done: true, Usage: usage}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// toUsage converts Gemini usage metadata to types.Usage.
func geminiToUsage(u *geminiUsage) types.Usage {
	if u == nil {
		return types.Usage{}
	}
	return types.Usage{
		PromptTokens:     u.PromptTokenCount,
		CompletionTokens: u.CandidatesTokenCount,
		TotalTokens:      u.TotalTokenCount,
	}
}
