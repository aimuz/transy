package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

const (
	// DefaultURL is the default OpenAI Realtime API URL
	DefaultURL = "wss://api.openai.com/v1/realtime"
	// DefaultModel is the default model to use
	DefaultModel = "gpt-4o-realtime-preview-2024-10-01"
)

// Client handles the WebSocket connection to OpenAI Realtime API.
type Client struct {
	url     string
	apiKey  string
	model   string
	conn    *websocket.Conn
	msgChan chan ClientEvent
	errChan chan error
	done    chan struct{}
	closed  bool
	mu      sync.Mutex
}

// ClientConfig holds configuration for the Realtime Client.
type ClientConfig struct {
	ApiKey string
	Model  string
	URL    string
}

// NewClient creates a new Realtime Client.
func NewClient(cfg ClientConfig) *Client {
	url := cfg.URL
	if url == "" {
		url = DefaultURL
	}
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	// Append model param if not present (simplified)
	if url == DefaultURL {
		url = fmt.Sprintf("%s?model=%s", url, model)
	}

	return &Client{
		url:     url,
		apiKey:  cfg.ApiKey,
		model:   model,
		msgChan: make(chan ClientEvent, 100),
		errChan: make(chan error, 1),
		done:    make(chan struct{}),
	}
}

// Connect establishes the WebSocket connection.
func (c *Client) Connect(ctx context.Context) error {
	opts := &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Authorization": {"Bearer " + c.apiKey},
			"OpenAI-Beta":   {"realtime=v1"},
		},
	}

	conn, _, err := websocket.Dial(ctx, c.url, opts)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}

	c.conn = conn

	// Start reading loop
	go c.readLoop()

	return nil
}

// Send sends an event to the API.
func (c *Client) Send(ctx context.Context, event interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	return c.conn.Write(ctx, websocket.MessageText, data)
}

// Receive returns a channel for receiving server events.
func (c *Client) Messages() <-chan ClientEvent {
	return c.msgChan
}

// Errors returns a channel for receiving errors.
func (c *Client) Errors() <-chan error {
	return c.errChan
}

// Close closes the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.done)

	if c.conn != nil {
		return c.conn.Close(websocket.StatusNormalClosure, "closing")
	}
	return nil
}

func (c *Client) readLoop() {
	defer close(c.msgChan)

	ctx := context.Background()

	for {
		select {
		case <-c.done:
			return
		default:
			_, data, err := c.conn.Read(ctx)
			if err != nil {
				c.errChan <- fmt.Errorf("read error: %w", err)
				return
			}

			var event ClientEvent
			if err := json.Unmarshal(data, &event); err != nil {
				slog.Error("failed to unmarshal event", "error", err, "data", string(data))
				continue
			}

			select {
			case c.msgChan <- event:
			case <-time.After(100 * time.Millisecond):
				slog.Warn("msg channel full, dropping event", "type", event.Type)
			}
		}
	}
}

// ClientEvent represents a message sent to or received from the API.
type ClientEvent struct {
	EventID string         `json:"event_id,omitempty"`
	Type    string         `json:"type"`
	Error   *RealtimeError `json:"error,omitempty"`
	// Store all other fields dynamically
	Extra map[string]interface{} `json:"-"`
}

// RealtimeError represents an error from the Realtime API
type RealtimeError struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling to capture all fields.
func (e *ClientEvent) UnmarshalJSON(data []byte) error {
	// First unmarshal into a map to get all fields
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// Then unmarshal type-specific fields
	type Alias ClientEvent
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Store extra fields
	e.Extra = make(map[string]interface{})
	for k, v := range rawMap {
		if k != "event_id" && k != "type" && k != "error" {
			e.Extra[k] = v
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Event Builders
// ─────────────────────────────────────────────────────────────────────────────

// EventSessionUpdate creates a session.update event.
func EventSessionUpdate(session map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":    "session.update",
		"session": session,
	}
}

// EventInputAudioBufferAppend creates an input_audio_buffer.append event.
func EventInputAudioBufferAppend(audioBase64 string) map[string]interface{} {
	return map[string]interface{}{
		"type":  "input_audio_buffer.append",
		"audio": audioBase64,
	}
}

// EventInputAudioBufferCommit creates an input_audio_buffer.commit event.
func EventInputAudioBufferCommit() map[string]interface{} {
	return map[string]interface{}{
		"type": "input_audio_buffer.commit",
	}
}

// EventResponseCreate creates a response.create event.
func EventResponseCreate(response map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":     "response.create",
		"response": response,
	}
}
