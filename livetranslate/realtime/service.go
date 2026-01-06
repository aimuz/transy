package realtime

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.aimuz.me/transy/audiocapture"
	"go.aimuz.me/transy/internal/types"
)

// Service provides real-time speech-to-speech/text execution using OpenAI Realtime API.
type Service struct {
	config ServiceConfig
	client *Client
	audio  *audiocapture.Capture

	// State
	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Callbacks (deprecated, use channels)
	onTranscript func(types.LiveTranscript)
	onError      func(error)

	// Channels for LiveTranslator interface
	transcriptChan chan types.LiveTranscript
	errorChan      chan error

	// Languages
	sourceLang string
	targetLang string

	// Accumulation
	currentTranscriptID   string
	currentTranscriptText string
	currentTranslation    string
}

// ServiceConfig holds configuration for the Realtime Service.
type ServiceConfig struct {
	APIKey       string
	Model        string
	SystemPrompt string
	Temperature  float64
}

// NewService creates a new Realtime Service.
func NewService(cfg ServiceConfig) (*Service, error) {
	// OpenAI Realtime API expects 24kHz audio
	audioCfg := audiocapture.DefaultConfig()
	audioCfg.SampleRate = 24000

	audioCap, err := audiocapture.New(audioCfg)
	if err != nil {
		return nil, fmt.Errorf("create audio capture: %w", err)
	}

	clientCfg := ClientConfig{
		ApiKey: cfg.APIKey,
		Model:  cfg.Model,
	}

	return &Service{
		config: cfg,
		client: NewClient(clientCfg),
		audio:  audioCap,
		// Initialize channels
		transcriptChan: make(chan types.LiveTranscript, 10),
		errorChan:      make(chan error, 10),
	}, nil
}

// Start begins the realtime session.
// Implements LiveTranslator interface.
func (s *Service) Start(ctx context.Context, sourceLang, targetLang string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("already running")
	}

	// Store langs if needed for prompt engineering later
	s.sourceLang = sourceLang
	s.targetLang = targetLang

	ctx, cancel := context.WithCancel(ctx)
	s.ctx = ctx
	s.cancel = cancel

	// Connect to Client
	if err := s.client.Connect(ctx); err != nil {
		cancel()
		return fmt.Errorf("connect client: %w", err)
	}

	// Start audio capture
	if err := s.audio.Start(); err != nil {
		s.client.Close()
		cancel()
		return fmt.Errorf("start audio capture: %w", err)
	}
	s.audio.OnAudio(s.handleAudio)

	s.running = true

	// Start processing events
	go s.processEvents()

	// Update Session
	go s.initSession()

	slog.Info("realtime service started")
	return nil
}

func (s *Service) initSession() {
	// Build translation instructions
	instructions := s.config.SystemPrompt
	if s.targetLang != "" && s.targetLang != "auto" {
		targetLangName := getLanguageName(s.targetLang)
		instructions = fmt.Sprintf(
			"You are a real-time speech translator. Listen to the audio input and translate it into %s. "+
				"Provide ONLY the translated text in your response, without any explanations or meta-commentary. "+
				"Maintain the original meaning, tone, and context.",
			targetLangName,
		)
	}

	sessionConfig := map[string]interface{}{
		"modalities":          []string{"text"}, // Only text for now to save bandwidth/complexity
		"instructions":        instructions,
		"input_audio_format":  "pcm16",
		"output_audio_format": "pcm16",
		"turn_detection": map[string]interface{}{
			"type": "server_vad",
		},
	}

	if s.config.Temperature > 0 {
		sessionConfig["temperature"] = s.config.Temperature
	}

	event := EventSessionUpdate(sessionConfig)
	if err := s.client.Send(s.ctx, event); err != nil {
		slog.Error("failed to update session", "error", err)
	}
}

// Stop ends the realtime session.
// Implements LiveTranslator interface.
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	if s.cancel != nil {
		s.cancel()
	}

	if s.audio != nil {
		if err := s.audio.Stop(); err != nil {
			slog.Error("stop audio", "error", err)
		}
	}

	if s.client != nil {
		if err := s.client.Close(); err != nil {
			slog.Error("close client", "error", err)
		}
	}

	// Close channels
	close(s.transcriptChan)
	close(s.errorChan)

	slog.Info("realtime service stopped")
	return nil
}

func (s *Service) handleAudio(samples []float32) {
	// Need to convert []float32 to PCM16 []byte for API
	pcm16 := float32ToPCM16(samples)
	base64Audio := base64.StdEncoding.EncodeToString(pcm16)

	event := EventInputAudioBufferAppend(base64Audio)

	// Send asynchronously to avoid blocking audio callback
	go func() {
		s.mu.RLock()
		ctx := s.ctx
		s.mu.RUnlock()

		if ctx == nil {
			return
		}

		if err := s.client.Send(ctx, event); err != nil {
			// Don't log every send error to avoid spam, or log debug
			// slog.Debug("failed to send audio", "error", err)
		}
	}()
}

func (s *Service) processEvents() {
	for event := range s.client.Messages() {
		switch event.Type {
		case "response.text.delta":
			s.handleTextDelta(event.Extra)
		case "response.text.done":
			s.handleTextDone(event.Extra)
		case "error":
			if event.Error != nil {
				slog.Error("realtime api error",
					"type", event.Error.Type,
					"code", event.Error.Code,
					"message", event.Error.Message,
					"param", event.Error.Param)
				if s.onError != nil {
					s.onError(fmt.Errorf("realtime api error [%s]: %s", event.Error.Code, event.Error.Message))
				}
			} else {
				slog.Error("realtime api error with no details")
			}
		}
	}
}

func (s *Service) handleTextDelta(extra map[string]interface{}) {
	delta, ok := extra["delta"].(string)
	if !ok {
		slog.Warn("text.delta event missing delta field")
		return
	}

	s.mu.Lock()
	s.currentTranscriptText += delta
	currentText := s.currentTranscriptText
	id := s.currentTranscriptID
	if id == "" {
		id = fmt.Sprintf("rt-%d", time.Now().UnixMilli())
		s.currentTranscriptID = id
	}
	s.mu.Unlock()

	s.emitTranscript(id, currentText, false)
}

func (s *Service) handleTextDone(extra map[string]interface{}) {
	text, ok := extra["text"].(string)
	if !ok {
		slog.Warn("text.done event missing text field")
		return
	}

	s.mu.Lock()
	// Overwrite with final text to be safe
	s.currentTranscriptText = text
	id := s.currentTranscriptID

	// Reset for next turn
	s.currentTranscriptID = ""
	s.currentTranscriptText = ""
	s.mu.Unlock()

	if id != "" {
		s.emitTranscript(id, text, true)
	}
}

func (s *Service) emitTranscript(id, text string, isFinal bool) {
	s.mu.RLock()
	cb := s.onTranscript
	s.mu.RUnlock()

	if cb != nil {
		cb(types.LiveTranscript{
			ID:        id,
			Text:      text,
			Timestamp: time.Now().UnixMilli(),
			IsFinal:   isFinal,
		})
	}
}

func (s *Service) OnTranscript(callback func(types.LiveTranscript)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTranscript = callback
}

func (s *Service) OnError(callback func(error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onError = callback
}

// Helpers

func float32ToPCM16(samples []float32) []byte {
	bytes := make([]byte, len(samples)*2)
	for i, sample := range samples {
		// Clamp to [-1, 1]
		if sample < -1 {
			sample = -1
		} else if sample > 1 {
			sample = 1
		}
		// Scale to int16 range
		val := int16(sample * 32767)
		bytes[i*2] = byte(val)
		bytes[i*2+1] = byte(val >> 8)
	}
	return bytes
}

func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Status returns the current status.
func (s *Service) Status() types.LiveStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return types.LiveStatus{
		Active:          s.running,
		SourceLang:      s.sourceLang,
		TargetLang:      s.targetLang,
		STTProvider:     "OpenAI Realtime API",
		Duration:        0, // TODO: calculate duration
		TranscriptCount: 0, // TODO: track count
	}
}

// Transcripts returns a read-only channel for receiving transcripts.
// Implements LiveTranslator interface.
func (s *Service) Transcripts() <-chan types.LiveTranscript {
	return s.transcriptChan
}

// Errors returns a read-only channel for receiving errors.
// Implements LiveTranslator interface.
func (s *Service) Errors() <-chan error {
	return s.errorChan
}

func (s *Service) Close() error {
	return s.Stop()
}

// getLanguageName returns the English name for a language code
func getLanguageName(code string) string {
	langMap := map[string]string{
		"zh":   "Chinese",
		"en":   "English",
		"ja":   "Japanese",
		"ko":   "Korean",
		"fr":   "French",
		"de":   "German",
		"es":   "Spanish",
		"ru":   "Russian",
		"it":   "Italian",
		"pt":   "Portuguese",
		"ar":   "Arabic",
		"auto": "the appropriate language",
	}
	if name, ok := langMap[code]; ok {
		return name
	}
	return code // fallback to code if not found
}
