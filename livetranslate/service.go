// Package livetranslate provides real-time audio translation service.
package livetranslate

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.aimuz.me/transy/audiocapture"
	"go.aimuz.me/transy/internal/types"
	"go.aimuz.me/transy/stt"
)

// TranslateFunc translates text from source to target language.
// context provides recent preceding text for better translation coherence.
type TranslateFunc func(text, context, srcLang, dstLang string) (string, error)

// Service coordinates audio capture, speech recognition, and translation.
// Refactored to follow Russ Cox's principles: simple, focused components.
type Service struct {
	// Dependencies
	audio       *audiocapture.Capture
	sttProvider stt.Provider
	translate   TranslateFunc

	// Components
	vad     *VAD
	buffer  *AudioBuffer
	manager *TranscriptManager

	// Configuration
	sourceLang string
	targetLang string

	// State
	mu        sync.RWMutex
	running   bool
	startTime time.Time

	// Channels for LiveTranslator interface
	transcriptChan chan types.LiveTranscript
	errorChan      chan error
	ctx            context.Context
	cancel         context.CancelFunc
}

// Config holds configuration for the live translation service.
type Config struct {
	STTProvider     stt.Provider
	TranslateFunc   TranslateFunc
	VADThreshold    float32
	MinSpeechDur    time.Duration
	MaxSpeechDur    time.Duration
	SilenceDur      time.Duration
	TranscribeDelay time.Duration
}

// DefaultConfig returns the default service configuration.
// Optimized for low-latency, Apple Live Captions-like experience.
func DefaultConfig() Config {
	return Config{
		VADThreshold:    0.015,
		MinSpeechDur:    300 * time.Millisecond,
		MaxSpeechDur:    5 * time.Second,
		SilenceDur:      400 * time.Millisecond,
		TranscribeDelay: 300 * time.Millisecond,
	}
}

// NewService creates a new live translation service.
func NewService(cfg Config) (*Service, error) {
	audioCap, err := audiocapture.New(audiocapture.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("create audio capture: %w", err)
	}

	// Apply defaults
	if cfg.VADThreshold == 0 {
		cfg.VADThreshold = 0.015
	}
	if cfg.MinSpeechDur == 0 {
		cfg.MinSpeechDur = 300 * time.Millisecond
	}
	if cfg.MaxSpeechDur == 0 {
		cfg.MaxSpeechDur = 5 * time.Second
	}
	if cfg.SilenceDur == 0 {
		cfg.SilenceDur = 400 * time.Millisecond
	}
	if cfg.TranscribeDelay == 0 {
		cfg.TranscribeDelay = 300 * time.Millisecond
	}

	// Create VAD
	vad := NewVAD(
		cfg.VADThreshold,
		cfg.MinSpeechDur,
		cfg.MaxSpeechDur,
		cfg.SilenceDur,
		cfg.TranscribeDelay,
	)

	// Create audio buffer with 0.5s overlap
	buffer := NewAudioBuffer(audioCap.SampleRate(), 0.5/float64(audioCap.SampleRate()))

	s := &Service{
		audio:          audioCap,
		sttProvider:    cfg.STTProvider,
		translate:      cfg.TranslateFunc,
		vad:            vad,
		buffer:         buffer,
		transcriptChan: make(chan types.LiveTranscript, 10),
		errorChan:      make(chan error, 10),
	}

	// Register audio callback
	s.audio.OnAudio(s.handleAudio)

	return s, nil
}

// SetSTTProvider sets the speech-to-text provider.
func (s *Service) SetSTTProvider(p stt.Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sttProvider = p
}

// SetTranslateFunc sets the translation function.
func (s *Service) SetTranslateFunc(f TranslateFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.translate = f
}

// Start begins live translation.
// Implements LiveTranslator interface.
func (s *Service) Start(ctx context.Context, sourceLang, targetLang string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("already running")
	}

	if s.sttProvider == nil {
		return fmt.Errorf("no STT provider set")
	}

	if !s.sttProvider.IsReady() {
		return fmt.Errorf("STT provider not ready")
	}

	// Create cancellable context
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.sourceLang = sourceLang
	s.targetLang = targetLang
	s.startTime = time.Now()

	// Initialize components
	s.vad.Reset()
	s.buffer.Clear()
	s.manager = NewTranscriptManager(
		s.startTime,
		sourceLang,
		targetLang,
		1*time.Second, // Merge threshold
		3,             // Max context
	)

	if err := s.audio.Start(); err != nil {
		return fmt.Errorf("start audio capture: %w", err)
	}

	s.running = true
	slog.Info("live translation started", "source", sourceLang, "target", targetLang)

	return nil
}

// Stop stops live translation.
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

	if err := s.audio.Stop(); err != nil {
		slog.Error("stop audio capture", "error", err)
	}

	// Close channels
	close(s.transcriptChan)
	close(s.errorChan)

	slog.Info("live translation stopped", "duration", time.Since(s.startTime))
	return nil
}

// IsRunning returns true if the service is running.
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Status returns the current status.
func (s *Service) Status() types.LiveStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providerName := ""
	if s.sttProvider != nil {
		providerName = s.sttProvider.Name()
	}

	var duration int64
	var count int
	if s.running {
		duration = int64(time.Since(s.startTime).Seconds())
		if s.manager != nil {
			count = s.manager.Count()
		}
	}

	return types.LiveStatus{
		Active:          s.running,
		SourceLang:      s.sourceLang,
		TargetLang:      s.targetLang,
		STTProvider:     providerName,
		Duration:        duration,
		TranscriptCount: count,
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

// handleAudio processes incoming audio samples.
func (s *Service) handleAudio(samples []float32) {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}

	// Process through VAD
	result := s.vad.Process(samples, s.audio.SampleRate())

	// Accumulate audio
	s.buffer.Append(samples)

	shouldTranscribe := result.ShouldTranscribe
	isFinal := result.Event.Type == EventSpeechEnd

	s.mu.Unlock()

	// Trigger transcription if needed
	if shouldTranscribe {
		s.transcribe(isFinal)
	}
}

// transcribe performs speech-to-text and translation.
func (s *Service) transcribe(isFinal bool) {
	s.mu.Lock()

	// Extract audio with timing
	audio, startTime, duration := s.buffer.ExtractWithDuration(s.startTime)
	if len(audio) == 0 {
		s.mu.Unlock()
		return
	}

	provider := s.sttProvider
	translateFn := s.translate
	sourceLang := s.sourceLang
	targetLang := s.targetLang
	manager := s.manager

	s.mu.Unlock()

	// Transcribe
	result, err := provider.Transcribe(audio, sourceLang)
	if err != nil {
		slog.Error("transcription failed", "error", err)
		s.sendError(err)
		return
	}

	// Clean and validate text
	text := cleanText(result.Text)
	if text == "" {
		return
	}

	// Process through manager
	processResult := manager.Process(text, startTime, duration, result.Confidence)
	segment := processResult.Segment

	// Emit initial transcript (without translation)
	s.sendTranscript(manager.ToLiveTranscript(segment))

	// Translate asynchronously if needed
	if translateFn != nil && targetLang != "" && sourceLang != targetLang {
		go s.translateSegment(segment, processResult.Context, translateFn, sourceLang, targetLang, manager, isFinal)
	} else if isFinal {
		manager.MarkFinal()
		s.sendTranscript(manager.ToLiveTranscript(segment))
	}
}

// translateSegment translates a segment and emits the updated transcript.
func (s *Service) translateSegment(segment *Segment, context string, translateFn TranslateFunc, sourceLang, targetLang string, manager *TranscriptManager, isFinal bool) {
	translated, err := translateFn(segment.SourceText, context, sourceLang, targetLang)
	if err != nil {
		slog.Error("translation failed", "error", err)
		s.sendError(err)
		translated = segment.SourceText // Fallback to source text
	}

	// Update segment
	manager.UpdateTranslation(segment.ID, translated)
	if isFinal {
		manager.MarkFinal()
	}

	// Emit updated transcript
	s.sendTranscript(manager.ToLiveTranscript(segment))
}

// sendTranscript sends a transcript to the channel (non-blocking).
func (s *Service) sendTranscript(t types.LiveTranscript) {
	select {
	case s.transcriptChan <- t:
	default:
		// Channel full, skip
	}
}

// sendError sends an error to the channel (non-blocking).
func (s *Service) sendError(err error) {
	select {
	case s.errorChan <- err:
	default:
		// Channel full, skip
	}
}

// Close releases resources.
func (s *Service) Close() error {
	return s.Stop()
}
