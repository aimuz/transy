// Package stt provides speech-to-text provider interface and implementations.
package stt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"time"
)

const (
	defaultWhisperAPIURL = "https://api.openai.com/v1/audio/transcriptions"
)

// WhisperAPI implements the Provider interface using OpenAI's Whisper API.
type WhisperAPI struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client

	mu    sync.RWMutex
	ready bool
}

// WhisperAPIConfig holds configuration for WhisperAPI.
type WhisperAPIConfig struct {
	APIKey  string
	BaseURL string // Optional, defaults to OpenAI's API
	Model   string // Optional, defaults to "whisper-1"
}

// NewWhisperAPI creates a new WhisperAPI provider.
func NewWhisperAPI(cfg WhisperAPIConfig) *WhisperAPI {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultWhisperAPIURL
	}

	model := cfg.Model
	if model == "" {
		model = "whisper-1"
	}

	w := &WhisperAPI{
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		model:   model,
		http:    &http.Client{Timeout: 60 * time.Second},
		ready:   cfg.APIKey != "",
	}

	return w
}

func (w *WhisperAPI) Name() string        { return "whisper-api" }
func (w *WhisperAPI) DisplayName() string { return "OpenAI Whisper API" }
func (w *WhisperAPI) IsLocal() bool       { return false }
func (w *WhisperAPI) RequiresSetup() bool { return false }
func (w *WhisperAPI) SetupProgress() int  { return 100 }

func (w *WhisperAPI) IsReady() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ready
}

func (w *WhisperAPI) Setup(_ func(percent int)) error {
	// No setup needed for API, just validate API key exists
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ready = w.apiKey != ""
	if !w.ready {
		return fmt.Errorf("API key is required")
	}
	return nil
}

// Transcribe sends audio to the Whisper API for transcription.
// audio: PCM float32 samples at 16000 Hz
// language: source language code (empty for auto-detect)
func (w *WhisperAPI) Transcribe(audio []float32, language string) (*TranscribeResult, error) {
	if !w.IsReady() {
		return nil, fmt.Errorf("WhisperAPI is not ready: API key required")
	}

	// Convert float32 PCM to WAV format
	wavData, err := float32ToWAV(audio, 16000)
	if err != nil {
		return nil, fmt.Errorf("convert to WAV: %w", err)
	}

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add audio file
	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(wavData); err != nil {
		return nil, fmt.Errorf("write audio data: %w", err)
	}

	// Add model
	if err := writer.WriteField("model", w.model); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}

	// Add language if specified (and not 'auto' which means auto-detect)
	// OpenAI API does not accept 'auto', empty means auto-detect
	if language != "" && language != "auto" {
		if err := writer.WriteField("language", language); err != nil {
			return nil, fmt.Errorf("write language field: %w", err)
		}
	}

	// Request timestamps for segments
	if err := writer.WriteField("response_format", "verbose_json"); err != nil {
		return nil, fmt.Errorf("write response_format field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", w.baseURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+w.apiKey)

	// Send request
	resp, err := w.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp whisperAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Convert to TranscribeResult
	result := &TranscribeResult{
		Text:       apiResp.Text,
		Language:   apiResp.Language,
		Confidence: 1.0, // API doesn't return confidence, assume high
		Segments:   make([]Segment, len(apiResp.Segments)),
	}

	for i, seg := range apiResp.Segments {
		result.Segments[i] = Segment{
			Text:  seg.Text,
			Start: time.Duration(seg.Start * float64(time.Second)),
			End:   time.Duration(seg.End * float64(time.Second)),
		}
	}

	return result, nil
}

func (w *WhisperAPI) Close() error {
	return nil
}

// whisperAPIResponse represents the Whisper API response.
type whisperAPIResponse struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Segments []struct {
		Text  string  `json:"text"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	} `json:"segments"`
}

// float32ToWAV converts float32 PCM samples to WAV format.
func float32ToWAV(samples []float32, sampleRate int) ([]byte, error) {
	// Convert float32 [-1, 1] to int16
	numSamples := len(samples)
	dataSize := numSamples * 2 // 16-bit = 2 bytes per sample

	buf := bytes.NewBuffer(make([]byte, 0, 44+dataSize))

	// RIFF header
	buf.WriteString("RIFF")
	writeUint32LE(buf, uint32(36+dataSize)) // File size - 8
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	writeUint32LE(buf, 16)                   // Chunk size
	writeUint16LE(buf, 1)                    // Audio format (PCM)
	writeUint16LE(buf, 1)                    // Num channels (mono)
	writeUint32LE(buf, uint32(sampleRate))   // Sample rate
	writeUint32LE(buf, uint32(sampleRate*2)) // Byte rate
	writeUint16LE(buf, 2)                    // Block align
	writeUint16LE(buf, 16)                   // Bits per sample

	// data chunk
	buf.WriteString("data")
	writeUint32LE(buf, uint32(dataSize))

	// Write samples
	for _, s := range samples {
		// Clamp to [-1, 1]
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		// Convert to int16
		sample := int16(s * 32767)
		writeInt16LE(buf, sample)
	}

	return buf.Bytes(), nil
}

func writeUint16LE(w *bytes.Buffer, v uint16) {
	w.WriteByte(byte(v))
	w.WriteByte(byte(v >> 8))
}

func writeUint32LE(w *bytes.Buffer, v uint32) {
	w.WriteByte(byte(v))
	w.WriteByte(byte(v >> 8))
	w.WriteByte(byte(v >> 16))
	w.WriteByte(byte(v >> 24))
}

func writeInt16LE(w *bytes.Buffer, v int16) {
	w.WriteByte(byte(v))
	w.WriteByte(byte(v >> 8))
}
