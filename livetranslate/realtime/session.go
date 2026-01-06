package realtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/realtime"
)

const (
	// RealtimeEndpoint is the endpoint for WebRTC SDP exchange
	RealtimeEndpoint = "https://api.openai.com/v1/realtime/calls"
)

// SessionManager handles creating ephemeral WebRTC sessions with OpenAI.
type SessionManager struct {
	client     *openai.Client
	httpClient *http.Client
}

// NewSessionManager creates a new session manager using the official OpenAI SDK.
func NewSessionManager(apiKey string) *SessionManager {
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &SessionManager{
		client: &client,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SessionConfig holds configuration for creating a session.
type SessionConfig struct {
	Language string // ISO-639-1 language code (e.g., "en", "zh")
}

// ClientSecretResult holds the ephemeral key from CreateSession.
type ClientSecretResult struct {
	Value     string
	ExpiresAt int64
}

// CreateSession creates a new ephemeral WebRTC transcription session token using the official SDK.
// This uses transcription-only mode (type: "transcription") - audio input, text output, no responses.
func (sm *SessionManager) CreateSession(ctx context.Context, cfg SessionConfig) (*ClientSecretResult, error) {
	// Build SDK params for transcription-only mode
	params := realtime.ClientSecretNewParams{
		Session: realtime.ClientSecretNewParamsSessionUnion{
			OfTranscription: &realtime.RealtimeTranscriptionSessionCreateRequestParam{
				// Configure input audio: transcription model and VAD
				Audio: realtime.RealtimeTranscriptionSessionAudioParam{
					Input: realtime.RealtimeTranscriptionSessionAudioInputParam{
						// Use server-side VAD for automatic turn detection
						TurnDetection: realtime.RealtimeTranscriptionSessionAudioInputTurnDetectionUnionParam{
							OfServerVad: &realtime.RealtimeTranscriptionSessionAudioInputTurnDetectionServerVadParam{
								Type:              "server_vad",
								Threshold:         openai.Float(0.5),
								PrefixPaddingMs:   openai.Int(300),
								SilenceDurationMs: openai.Int(500),
							},
						},
						// Configure transcription
						Transcription: realtime.AudioTranscriptionParam{
							Model:    realtime.AudioTranscriptionModelGPT4oTranscribe,
							Language: openai.String(cfg.Language),
						},
					},
				},
			},
		},
	}

	// Call the SDK
	resp, err := sm.client.Realtime.ClientSecrets.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create client secret: %w", err)
	}

	// The client secret value is on the top-level response, not inside Session
	return &ClientSecretResult{
		Value:     resp.Value,
		ExpiresAt: resp.ExpiresAt,
	}, nil
}

// ExchangeSDP sends the local SDP offer to OpenAI and receives the SDP answer.
// The ephemeralKey should be the ClientSecretResult.Value from CreateSession.
// Note: The SDK does not support WebRTC SDP exchange, so we do this manually.
func (sm *SessionManager) ExchangeSDP(ctx context.Context, offer string, ephemeralKey string) (string, error) {
	// Debug: log key info (first 20 chars for security)
	keyLen := len(ephemeralKey)
	keyPreview := ephemeralKey
	if keyLen > 20 {
		keyPreview = ephemeralKey[:20] + "..."
	}
	slog.Debug("ExchangeSDP", "ephemeralKeyLen", keyLen, "keyPreview", keyPreview)

	// Create HTTP request with SDP offer as plain text body
	req, err := http.NewRequestWithContext(ctx, "POST", RealtimeEndpoint, bytes.NewBufferString(offer))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// Use ephemeral key for authentication
	req.Header.Set("Authorization", "Bearer "+ephemeralKey)
	req.Header.Set("Content-Type", "application/sdp")

	// Execute request
	resp, err := sm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Check status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		slog.Error("SDP exchange failed", "status", resp.StatusCode, "body", string(body))
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// The response should be the SDP answer as plain text
	return string(body), nil
}
