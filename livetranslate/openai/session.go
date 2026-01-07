package openai

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
	// RealtimeEndpoint is the endpoint for WebRTC SDP exchange.
	RealtimeEndpoint = "https://api.openai.com/v1/realtime/calls"
)

// SessionToken holds the ephemeral key from CreateSession.
type SessionToken struct {
	Value     string
	ExpiresAt int64
}

// httpClient is a package-level client with connection reuse.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// SessionConfig holds configuration for creating a transcription session.
type SessionConfig struct {
	Model    string // Transcription model, e.g. "gpt-4o-transcribe-diarize"
	Language string // Language code, e.g. "en"
	Prompt   string // Optional transcription prompt
}

// CreateSession creates a new ephemeral WebRTC transcription session token.
func CreateSession(ctx context.Context, apiKey string, cfg SessionConfig) (*SessionToken, error) {
	language := cfg.Language
	if language == "" {
		language = "en"
	}
	model := cfg.Model
	if model == "" {
		model = string(realtime.AudioTranscriptionModelGPT4oTranscribe)
	}

	client := openai.NewClient(option.WithAPIKey(apiKey))

	transcription := realtime.AudioTranscriptionParam{
		Model:    realtime.AudioTranscriptionModel(model),
		Language: openai.String(language),
	}
	if cfg.Prompt != "" {
		transcription.Prompt = openai.String(cfg.Prompt)
	}

	params := realtime.ClientSecretNewParams{
		Session: realtime.ClientSecretNewParamsSessionUnion{
			OfTranscription: &realtime.RealtimeTranscriptionSessionCreateRequestParam{
				Audio: realtime.RealtimeTranscriptionSessionAudioParam{
					Input: realtime.RealtimeTranscriptionSessionAudioInputParam{
						TurnDetection: realtime.RealtimeTranscriptionSessionAudioInputTurnDetectionUnionParam{
							OfSemanticVad: &realtime.RealtimeTranscriptionSessionAudioInputTurnDetectionSemanticVadParam{
								Type:      "semantic_vad",
								Eagerness: string(VADEagernessHigh),
							},
						},
						Transcription: transcription,
					},
				},
			},
		},
	}
	resp, err := client.Realtime.ClientSecrets.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create client secret: %w", err)
	}

	return &SessionToken{
		Value:     resp.Value,
		ExpiresAt: resp.ExpiresAt,
	}, nil
}

// ExchangeSDP sends the local SDP offer to OpenAI and receives the SDP answer.
func ExchangeSDP(ctx context.Context, offer, ephemeralKey string) (string, error) {
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		keyPreview := ephemeralKey
		if len(keyPreview) > 20 {
			keyPreview = keyPreview[:20] + "..."
		}
		slog.Debug("ExchangeSDP", "ephemeralKeyLen", len(ephemeralKey), "keyPreview", keyPreview)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, RealtimeEndpoint, bytes.NewBufferString(offer))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+ephemeralKey)
	req.Header.Set("Content-Type", "application/sdp")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		slog.Error("SDP exchange failed", "status", resp.StatusCode, "body", string(body))
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, body)
	}

	return string(body), nil
}
