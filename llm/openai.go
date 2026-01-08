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

const defaultBaseURL = "https://api.openai.com/v1/chat/completions"

// openaiCompleter implements Completer for OpenAI and compatible APIs.
type openaiCompleter struct {
	cfg          completerConfig
	isCompatible bool
}

// OpenAI request/response types
type openaiRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

type openaiMessage struct {
	Content string `json:"content"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (c *openaiCompleter) Complete(ctx context.Context, messages []Message) (string, types.Usage, error) {
	url := defaultBaseURL
	if c.isCompatible && c.cfg.baseURL != "" {
		url = c.cfg.baseURL
	}

	reqBody := openaiRequest{
		Model:       c.cfg.model,
		Messages:    messages,
		MaxTokens:   c.cfg.maxTokens,
		Temperature: c.cfg.temperature,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.apiKey)

	resp, err := c.cfg.http.Do(req)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", types.Usage{}, fmt.Errorf("api error: %d - %s", resp.StatusCode, string(body))
	}

	var chatResp openaiResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", types.Usage{}, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", types.Usage{}, fmt.Errorf("no choices")
	}

	usage := types.Usage{
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalTokens:      chatResp.Usage.TotalTokens,
	}

	return chatResp.Choices[0].Message.Content, usage, nil
}
