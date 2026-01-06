package realtime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.aimuz.me/transy/audiocapture"
	"go.aimuz.me/transy/internal/types"
)

// Service provides real-time speech-to-speech/text execution using OpenAI Realtime API via WebRTC.
type Service struct {
	config ServiceConfig
	client *Client // WebRTC client
	audio  *audiocapture.Capture

	// State
	mu        sync.RWMutex
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time
	count     int

	// Channels for LiveTranslator interface
	transcriptChan chan types.LiveTranscript
	errorChan      chan error

	// Languages
	sourceLang string
	targetLang string

	// Current segment accumulation
	currentSegmentID    string
	currentSegmentStart int64 // ms since session start
	currentSourceText   string
	currentTargetText   string
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
	// WebRTC Opus uses 48kHz - capture at native rate to avoid resampling
	audioCfg := audiocapture.DefaultConfig()
	audioCfg.SampleRate = 48000

	audioCap, err := audiocapture.New(audioCfg)
	if err != nil {
		return nil, fmt.Errorf("create audio capture: %w", err)
	}

	// Create WebRTC client
	client, err := NewClient(Config{
		APIKey: cfg.APIKey,
		Model:  cfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	return &Service{
		config: cfg,
		client: client,
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

	// Store session info
	s.sourceLang = sourceLang
	s.targetLang = targetLang
	s.startTime = time.Now()
	s.count = 0

	ctx, cancel := context.WithCancel(ctx)
	s.ctx = ctx
	s.cancel = cancel

	// Set up callback to initialize session when data channel is ready
	s.client.OnDataChannelOpen(func() {
		slog.Info("data channel ready, initializing session")
		s.initSession()
	})

	// Connect to OpenAI via WebRTC
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

	slog.Info("realtime service started (WebRTC)")
	return nil
}

func (s *Service) initSession() {
	// NOTE: For transcription-only sessions, there's no session.update needed.
	// The session config (audio format, transcription model, VAD) is set during
	// client secret creation. Instructions/temperature are for conversation mode only.
	//
	// If we need to switch to conversation mode (for translation), we would use
	// OfRealtime instead of OfTranscription in session.go and add instructions here.
	slog.Info("transcription session ready - no session.update needed for transcription mode")
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
	// Check audio level every 50 samples
	s.mu.Lock()
	s.count++
	shouldLog := s.count%50 == 0
	s.mu.Unlock()

	if shouldLog {
		// Calculate max amplitude (float32 range is -1.0 to 1.0)
		var maxAmp float32
		for _, sample := range samples {
			if sample < 0 {
				sample = -sample
			}
			if sample > maxAmp {
				maxAmp = sample
			}
		}
		// Convert to dB-like scale for logging (0-32767 range for comparison)
		slog.Info("audio status", "samples", len(samples), "max_amplitude", int(maxAmp*32767))
	} else {
		slog.Debug("sending audio", "samples", len(samples))
	}

	// Send float32 samples directly to WebRTC client (no int16 conversion)
	// Opus encoder is not thread-safe, so this must be synchronous
	if err := s.client.SendAudio(samples); err != nil {
		slog.Warn("failed to send audio", "error", err)
	}
}

func (s *Service) processEvents() {
	for event := range s.client.Messages() {
		slog.Info("received event", "type", event.Type, "extra", event.Extra)

		switch event.Type {
		case "conversation.item.input_audio_transcription.completed":
			// Source text received (transcription of user's speech)
			s.handleSourceTranscription(event.Extra)
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
				s.sendError(fmt.Errorf("realtime api error [%s]: %s", event.Error.Code, event.Error.Message))
			} else {
				slog.Error("realtime api error with no details")
			}
		default:
			slog.Debug("unhandled event type", "type", event.Type)
		}
	}
}

func (s *Service) handleSourceTranscription(extra map[string]interface{}) {
	transcript, ok := extra["transcript"].(string)
	if !ok || transcript == "" {
		return
	}

	s.mu.Lock()
	if s.currentSegmentID == "" {
		s.currentSegmentID = fmt.Sprintf("rt-%d", time.Now().UnixMilli())
		s.currentSegmentStart = time.Since(s.startTime).Milliseconds()
	}
	id := s.currentSegmentID
	startTime := s.currentSegmentStart
	endTime := time.Since(s.startTime).Milliseconds()

	// Reset for next segment
	s.currentSegmentID = ""
	s.currentSegmentStart = 0
	s.count++
	s.mu.Unlock()

	slog.Debug("source transcription", "text", transcript)

	// For transcription-only mode, the transcription IS the final output
	// Emit it directly to the frontend
	s.emitTranscript(id, transcript, transcript, startTime, endTime, true)
}

func (s *Service) handleTextDelta(extra map[string]interface{}) {
	delta, ok := extra["delta"].(string)
	if !ok {
		slog.Warn("text.delta event missing delta field")
		return
	}

	s.mu.Lock()
	s.currentTargetText += delta
	id := s.currentSegmentID
	if id == "" {
		id = fmt.Sprintf("rt-%d", time.Now().UnixMilli())
		s.currentSegmentID = id
		s.currentSegmentStart = time.Since(s.startTime).Milliseconds()
	}
	sourceText := s.currentSourceText
	targetText := s.currentTargetText
	startTime := s.currentSegmentStart
	s.mu.Unlock()

	s.emitTranscript(id, sourceText, targetText, startTime, 0, false)
}

func (s *Service) handleTextDone(extra map[string]interface{}) {
	text, ok := extra["text"].(string)
	if !ok {
		slog.Warn("text.done event missing text field")
		return
	}

	s.mu.Lock()
	s.currentTargetText = text
	id := s.currentSegmentID
	sourceText := s.currentSourceText
	startTime := s.currentSegmentStart

	// Calculate end time
	endTime := time.Since(s.startTime).Milliseconds()

	// Reset for next segment
	s.currentSegmentID = ""
	s.currentSourceText = ""
	s.currentTargetText = ""
	s.currentSegmentStart = 0
	s.count++
	s.mu.Unlock()

	if id != "" {
		s.emitTranscript(id, sourceText, text, startTime, endTime, true)
	}
}

func (s *Service) emitTranscript(id, sourceText, targetText string, startTime, endTime int64, isFinal bool) {
	s.mu.RLock()
	sourceLang := s.sourceLang
	targetLang := s.targetLang
	s.mu.RUnlock()

	transcript := types.LiveTranscript{
		ID:         id,
		SourceText: sourceText,
		TargetText: targetText,
		SourceLang: sourceLang,
		TargetLang: targetLang,
		StartTime:  startTime,
		EndTime:    endTime,
		Timestamp:  time.Now().UnixMilli(),
		IsFinal:    isFinal,
		Confidence: 1.0, // Realtime API doesn't provide confidence
		// Backward compatibility
		Text:       sourceText,
		Translated: targetText,
	}

	s.sendTranscript(transcript)
}

func (s *Service) sendTranscript(t types.LiveTranscript) {
	select {
	case s.transcriptChan <- t:
	default:
		// Channel full, skip
	}
}

func (s *Service) sendError(err error) {
	select {
	case s.errorChan <- err:
	default:
		// Channel full, skip
	}
}

// float32ToPCM16Samples converts float32 samples to int16 samples for WebRTC.
func float32ToPCM16Samples(samples []float32) []int16 {
	pcm16 := make([]int16, len(samples))
	for i, sample := range samples {
		// Clamp to [-1, 1]
		if sample < -1 {
			sample = -1
		} else if sample > 1 {
			sample = 1
		}
		// Scale to int16 range
		pcm16[i] = int16(sample * 32767)
	}
	return pcm16
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

	var duration int64
	if s.running {
		duration = int64(time.Since(s.startTime).Seconds())
	}

	return types.LiveStatus{
		Active:          s.running,
		SourceLang:      s.sourceLang,
		TargetLang:      s.targetLang,
		STTProvider:     "OpenAI Realtime API (WebRTC)",
		Duration:        duration,
		TranscriptCount: s.count,
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
