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
	Stream    bool            `json:"stream,omitempty"`
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

// Streaming response types
type claudeStreamEvent struct {
	Type  string          `json:"type"`
	Index int             `json:"index,omitempty"`
	Delta *claudeSSEDelta `json:"delta,omitempty"`
	Usage *claudeUsage    `json:"usage,omitempty"`
}

type claudeSSEDelta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

// buildRequest constructs the Claude API request body from messages.
func (c *claudeCompleter) buildRequest(messages []Message, stream bool) claudeRequest {
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

	return claudeRequest{
		Model:     c.cfg.model,
		Messages:  claudeMsgs,
		System:    systemPrompt,
		MaxTokens: maxTokens,
		Stream:    stream,
	}
}

// baseURL returns the configured or default base URL.
func (c *claudeCompleter) baseURL() string {
	if c.cfg.baseURL != "" {
		return c.cfg.baseURL
	}
	return defaultClaudeBaseURL
}

// newRequest creates an HTTP request with Claude-specific headers.
func (c *claudeCompleter) newRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL(), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("x-api-key", c.cfg.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	return req, nil
}

func (c *claudeCompleter) Complete(ctx context.Context, messages []Message) (string, types.Usage, error) {
	reqBody := c.buildRequest(messages, false)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", types.Usage{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := c.newRequest(ctx, jsonBody)
	if err != nil {
		return "", types.Usage{}, err
	}

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

	return claudeResp.Content[0].Text, claudeToUsage(claudeResp.Usage), nil
}

// StreamComplete implements StreamCompleter for streaming responses.
func (c *claudeCompleter) StreamComplete(ctx context.Context, messages []Message) (<-chan StreamDelta, error) {
	reqBody := c.buildRequest(messages, true)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := c.newRequest(ctx, jsonBody)
	if err != nil {
		return nil, err
	}

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

			if line == "" || strings.HasPrefix(line, "event:") || !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			var event claudeStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_delta":
				if event.Delta != nil && event.Delta.Text != "" {
					select {
					case ch <- StreamDelta{Text: event.Delta.Text}:
					case <-ctx.Done():
						return
					}
				}
			case "message_delta":
				if event.Usage != nil {
					usage = types.Usage{
						CompletionTokens: event.Usage.OutputTokens,
					}
				}
			case "message_stop":
				select {
				case ch <- StreamDelta{Done: true, Usage: usage}:
				case <-ctx.Done():
				}
				return
			}
		}

		select {
		case ch <- StreamDelta{Done: true, Usage: usage}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// claudeToUsage converts Claude usage to types.Usage.
func claudeToUsage(u *claudeUsage) types.Usage {
	if u == nil {
		return types.Usage{}
	}
	return types.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.InputTokens + u.OutputTokens,
	}
}
