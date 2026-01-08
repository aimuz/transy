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

const defaultBaseURL = "https://api.openai.com/v1/chat/completions"

// openaiCompleter implements Completer for OpenAI and compatible APIs.
type openaiCompleter struct {
	cfg          completerConfig
	isCompatible bool
}

// OpenAI request/response types
type openaiRequest struct {
	Model         string            `json:"model"`
	Messages      []Message         `json:"messages"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Temperature   float64           `json:"temperature,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	StreamOptions *openaiStreamOpts `json:"stream_options,omitempty"`
}

type openaiStreamOpts struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
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

// Streaming response types
type openaiStreamResponse struct {
	Choices []openaiStreamChoice `json:"choices"`
	Usage   *openaiUsage         `json:"usage,omitempty"`
}

type openaiStreamChoice struct {
	Delta        openaiStreamDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

type openaiStreamDelta struct {
	Content string `json:"content"`
}

// baseURL returns the configured or default base URL.
func (c *openaiCompleter) baseURL() string {
	if c.isCompatible && c.cfg.baseURL != "" {
		return c.cfg.baseURL
	}
	return defaultBaseURL
}

// buildRequest constructs the OpenAI API request body.
func (c *openaiCompleter) buildRequest(messages []Message, stream bool) openaiRequest {
	req := openaiRequest{
		Model:       c.cfg.model,
		Messages:    messages,
		MaxTokens:   c.cfg.maxTokens,
		Temperature: c.cfg.temperature,
		Stream:      stream,
	}
	if stream {
		req.StreamOptions = &openaiStreamOpts{IncludeUsage: true}
	}
	return req
}

// newRequest creates an HTTP request with OpenAI-specific headers.
func (c *openaiCompleter) newRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL(), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.apiKey)
	return req, nil
}

func (c *openaiCompleter) Complete(ctx context.Context, messages []Message) (string, types.Usage, error) {
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

	return chatResp.Choices[0].Message.Content, openaiToUsage(&chatResp.Usage), nil
}

// StreamComplete implements StreamCompleter for streaming responses.
func (c *openaiCompleter) StreamComplete(ctx context.Context, messages []Message) (<-chan StreamDelta, error) {
	reqBody := c.buildRequest(messages, true)

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := c.newRequest(ctx, jsonBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

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

			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// Check for stream end
			if data == "[DONE]" {
				select {
				case ch <- StreamDelta{Done: true, Usage: usage}:
				case <-ctx.Done():
				}
				return
			}

			var chunk openaiStreamResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case ch <- StreamDelta{Text: chunk.Choices[0].Delta.Content}:
				case <-ctx.Done():
					return
				}
			}

			if chunk.Usage != nil {
				usage = openaiToUsage(chunk.Usage)
			}
		}

		select {
		case ch <- StreamDelta{Done: true, Usage: usage}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// openaiToUsage converts OpenAI usage to types.Usage.
func openaiToUsage(u *openaiUsage) types.Usage {
	if u == nil {
		return types.Usage{}
	}
	return types.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}
