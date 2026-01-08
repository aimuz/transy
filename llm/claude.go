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

const defaultClaudeBaseURL = "https://api.anthropic.com/v1/messages"

// claudeCompleter implements Completer for Claude API.
type claudeCompleter struct {
	cfg completerConfig
}

// Claude request/response types
type claudeRequest struct {
	Model     string          `json:"model"`
	Messages  []claudeMessage `json:"messages"`
	System    string          `json:"system,omitempty"`
	MaxTokens int             `json:"max_tokens"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []claudeContent `json:"content"`
	Usage   *claudeUsage    `json:"usage,omitempty"`
	Error   *claudeError    `json:"error,omitempty"`
}

type claudeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (c *claudeCompleter) Complete(ctx context.Context, messages []Message) (string, types.Usage, error) {
	var claudeMsgs []claudeMessage
	var systemPrompt string

	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt += msg.Content
			continue
		}
		claudeMsgs = append(claudeMsgs, claudeMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	maxTokens := c.cfg.maxTokens
	if maxTokens == 0 {
		maxTokens = 1024 // Claude requires max_tokens
	}

	reqBody := claudeRequest{
		Model:     c.cfg.model,
		Messages:  claudeMsgs,
		System:    systemPrompt,
		MaxTokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("marshal request: %w", err)
	}

	baseURL := defaultClaudeBaseURL
	if c.cfg.baseURL != "" {
		baseURL = c.cfg.baseURL
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("x-api-key", c.cfg.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.cfg.http.Do(req)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("read response: %w", err)
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", types.Usage{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if claudeResp.Error != nil {
		return "", types.Usage{}, fmt.Errorf("api error: %s - %s", claudeResp.Error.Type, claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return "", types.Usage{}, fmt.Errorf("no content returned")
	}

	var usage types.Usage
	if claudeResp.Usage != nil {
		usage = types.Usage{
			PromptTokens:     claudeResp.Usage.InputTokens,
			CompletionTokens: claudeResp.Usage.OutputTokens,
			TotalTokens:      claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
		}
	}

	return claudeResp.Content[0].Text, usage, nil
}
