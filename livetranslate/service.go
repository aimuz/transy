// Package livetranslate provides real-time audio translation service.
package livetranslate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.aimuz.me/transy/audiocapture"
	"go.aimuz.me/transy/internal/types"
	"go.aimuz.me/transy/stt"
)

// TranslateFunc is a function that translates text from one language to another.
// context is the recent preceding text for better translation coherence.
type TranslateFunc func(text, context, srcLang, dstLang string) (string, error)

// Service coordinates audio capture, speech recognition, and translation.
type Service struct {
	audio       *audiocapture.Capture
	sttProvider stt.Provider
	translate   TranslateFunc

	// Configuration
	sourceLang string
	targetLang string

	// VAD (Voice Activity Detection) settings
	vadThreshold    float32       // RMS threshold for speech detection
	minSpeechDur    time.Duration // Minimum speech duration before transcription
	maxSpeechDur    time.Duration // Maximum speech duration before forced transcription
	silenceDur      time.Duration // Silence duration to end speech
	transcribeDelay time.Duration // Delay between transcriptions

	// State
	mu              sync.RWMutex
	running         bool
	startTime       time.Time
	transcriptID    atomic.Uint64
	transcriptCount int

	// Audio accumulator
	audioBuffer    []float32
	speechStart    time.Time
	lastSpeech     time.Time
	inSpeech       bool
	lastTranscribe time.Time

	// Segment merging state
	lastTranscriptID   string
	lastTranscriptText string
	lastTranscriptEnd  time.Time
	mergeThreshold     time.Duration // Time threshold to merge segments (e.g., 1s)

	// Context for translation (recent texts for coherence)
	recentTexts []string
	maxContext  int // Maximum number of recent texts to keep

	// Callbacks (deprecated, use channels)
	onTranscript func(types.LiveTranscript)
	onError      func(error)

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
	VADThreshold    float32       // Default: 0.02
	MinSpeechDur    time.Duration // Default: 500ms
	MaxSpeechDur    time.Duration // Default: 10s
	SilenceDur      time.Duration // Default: 1s
	TranscribeDelay time.Duration // Default: 500ms
}

// DefaultConfig returns the default service configuration.
// These values are optimized for low-latency, Apple Live Captions-like experience.
func DefaultConfig() Config {
	return Config{
		VADThreshold:    0.015,                  // Lower threshold for faster detection
		MinSpeechDur:    300 * time.Millisecond, // Minimum 300ms before first transcription
		MaxSpeechDur:    5 * time.Second,        // Force transcription every 5s for long speech
		SilenceDur:      400 * time.Millisecond, // End speech after 400ms silence
		TranscribeDelay: 300 * time.Millisecond, // Minimum 300ms between transcriptions
	}
}

// NewService creates a new live translation service.
func NewService(cfg Config) (*Service, error) {
	audioCap, err := audiocapture.New(audiocapture.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("create audio capture: %w", err)
	}

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

	s := &Service{
		audio:           audioCap,
		sttProvider:     cfg.STTProvider,
		translate:       cfg.TranslateFunc,
		vadThreshold:    cfg.VADThreshold,
		minSpeechDur:    cfg.MinSpeechDur,
		maxSpeechDur:    cfg.MaxSpeechDur,
		silenceDur:      cfg.SilenceDur,
		transcribeDelay: cfg.TranscribeDelay,
		audioBuffer:     make([]float32, 0, 16000*30), // 30 seconds buffer
		// Initialize channels
		transcriptChan: make(chan types.LiveTranscript, 10), // Buffered to avoid blocking
		errorChan:      make(chan error, 10),
	}

	// Register audio callback
	s.audio.OnAudio(s.handleAudio)

	s.maxContext = 3                   // Keep last 3 sentences for context
	s.mergeThreshold = 1 * time.Second // Merge segments if gap < 1s

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
	s.transcriptCount = 0
	s.audioBuffer = s.audioBuffer[:0]
	s.recentTexts = s.recentTexts[:0] // Clear context
	s.lastTranscriptID = ""
	s.lastTranscriptText = ""
	s.lastTranscriptEnd = time.Time{}
	s.inSpeech = false

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

	// Cancel context
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
	if s.running {
		duration = int64(time.Since(s.startTime).Seconds())
	}

	return types.LiveStatus{
		Active:          s.running,
		SourceLang:      s.sourceLang,
		TargetLang:      s.targetLang,
		STTProvider:     providerName,
		Duration:        duration,
		TranscriptCount: s.transcriptCount,
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

// OnTranscript sets the callback for new transcripts.
func (s *Service) OnTranscript(callback func(types.LiveTranscript)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTranscript = callback
}

// OnError sets the callback for errors.
func (s *Service) OnError(callback func(error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onError = callback
}

// handleAudio processes incoming audio samples.
func (s *Service) handleAudio(samples []float32) {
	// ... (unchanged)
	s.mu.Lock()

	if !s.running {
		s.mu.Unlock()
		return
	}

	now := time.Now()

	// Calculate RMS for voice activity detection
	rms := calculateRMS(samples)
	isSpeech := rms > s.vadThreshold

	if isSpeech {
		if !s.inSpeech {
			// Speech started
			s.inSpeech = true
			s.speechStart = now
		}
		s.lastSpeech = now
	}

	// Accumulate audio
	s.audioBuffer = append(s.audioBuffer, samples...)

	// Check if we should transcribe
	shouldTranscribe := false

	if s.inSpeech {
		speechDuration := now.Sub(s.speechStart)
		silenceDuration := now.Sub(s.lastSpeech)

		// Transcribe if:
		// 1. Speech ended (silence detected) and minimum duration met
		// 2. Maximum duration reached
		// 3. Enough time passed since last transcription
		if silenceDuration > s.silenceDur && speechDuration > s.minSpeechDur {
			shouldTranscribe = true
			s.inSpeech = false
		} else if speechDuration > s.maxSpeechDur {
			shouldTranscribe = true
		}
	}

	// Rate limit transcriptions
	if shouldTranscribe && now.Sub(s.lastTranscribe) < s.transcribeDelay {
		shouldTranscribe = false
	}

	if shouldTranscribe {
		// Copy buffer for async processing
		audioData := make([]float32, len(s.audioBuffer))
		copy(audioData, s.audioBuffer)

		// Clear buffer but keep some overlap for context
		overlapSamples := int(float64(s.audio.SampleRate()) * 0.5) // 0.5 second overlap
		if len(s.audioBuffer) > overlapSamples {
			copy(s.audioBuffer, s.audioBuffer[len(s.audioBuffer)-overlapSamples:])
			s.audioBuffer = s.audioBuffer[:overlapSamples]
		} else {
			s.audioBuffer = s.audioBuffer[:0]
		}

		s.lastTranscribe = now
		s.mu.Unlock()

		// Process in background
		go s.transcribeAndTranslate(audioData, !s.inSpeech)
		return
	}

	s.mu.Unlock()
}

// transcribeAndTranslate performs STT and translation.
// It first emits the transcription immediately, then translates asynchronously.
func (s *Service) transcribeAndTranslate(audio []float32, isFinal bool) {
	s.mu.RLock()
	provider := s.sttProvider
	translateFn := s.translate
	sourceLang := s.sourceLang
	targetLang := s.targetLang
	// Support both callback and channel patterns
	onTranscript := s.onTranscript
	onError := s.onError
	transcriptChan := s.transcriptChan
	errorChan := s.errorChan
	// Get recent context
	contextTexts := make([]string, len(s.recentTexts))
	copy(contextTexts, s.recentTexts)
	s.mu.RUnlock()

	if provider == nil || len(audio) == 0 {
		return
	}

	// Transcribe
	result, err := provider.Transcribe(audio, sourceLang)
	if err != nil {
		slog.Error("transcription failed", "error", err)
		// Send to both callback and channel
		if onError != nil {
			onError(err)
		}
		select {
		case errorChan <- err:
		default:
			// Channel full, skip
		}
		return
	}

	// Clean text (remove timestamps like [00:00:00.000 --> 00:00:04.000])
	result.Text = cleanText(result.Text)

	if result.Text == "" {
		return
	}

	s.mu.Lock()
	now := time.Now()
	timestamp := now.UnixMilli()

	// Determine if we should merge with the previous segment
	shouldMerge := false
	if s.lastTranscriptID != "" {
		gap := now.Sub(s.lastTranscriptEnd)
		if gap < s.mergeThreshold {
			shouldMerge = true
		}
	}

	var transcriptID string
	var fullText string

	if shouldMerge {
		// Merge with previous instead of creating new
		transcriptID = s.lastTranscriptID
		fullText = s.lastTranscriptText + " " + result.Text
		slog.Info("merging segment", "gap", now.Sub(s.lastTranscriptEnd), "id", transcriptID, "text", result.Text)

		// Update recent context: replace last entry with merged text
		if len(s.recentTexts) > 0 {
			s.recentTexts[len(s.recentTexts)-1] = fullText
		}
	} else {
		// Create new segment
		id := s.transcriptID.Add(1)
		transcriptID = fmt.Sprintf("lt-%d", id)
		fullText = result.Text

		// Add to recent context
		s.recentTexts = append(s.recentTexts, fullText)
		if len(s.recentTexts) > s.maxContext {
			s.recentTexts = s.recentTexts[1:]
		}
		s.transcriptCount++
	}

	// Update state
	s.lastTranscriptID = transcriptID
	s.lastTranscriptText = fullText
	s.lastTranscriptEnd = now
	s.mu.Unlock()

	// Emit transcript immediately (pending translation)
	transcript := types.LiveTranscript{
		ID:         transcriptID,
		Text:       fullText,
		Translated: "", // Translation pending
		Timestamp:  timestamp,
		IsFinal:    false,
		Confidence: result.Confidence,
	}

	// Send to both callback and channel
	if onTranscript != nil {
		onTranscript(transcript)
	}
	select {
	case transcriptChan <- transcript:
	default:
		// Channel full, skip
	}

	// Translate asynchronously
	go func() {
		translated := fullText
		if translateFn != nil && targetLang != "" && sourceLang != targetLang {
			// Join context texts
			s.mu.RLock()
			currentContext := make([]string, 0, len(s.recentTexts))
			for _, t := range s.recentTexts {
				if t != fullText { // Exclude self from context
					currentContext = append(currentContext, t)
				}
			}
			s.mu.RUnlock()

			contextStr := strings.Join(currentContext, " ")

			var tErr error
			translated, tErr = translateFn(fullText, contextStr, sourceLang, targetLang)
			if tErr != nil {
				slog.Error("translation failed", "error", tErr)
				if onError != nil {
					onError(tErr)
				}
				select {
				case errorChan <- tErr:
				default:
				}
				// Use original text if translation fails
				translated = fullText
			}
		}

		// Emit updated transcript with translation
		finalTranscript := types.LiveTranscript{
			ID:         transcriptID,
			Text:       fullText,
			Translated: translated,
			Timestamp:  time.Now().UnixMilli(),
			IsFinal:    isFinal,
			Confidence: result.Confidence,
		}

		// Send to both callback and channel
		if onTranscript != nil {
			onTranscript(finalTranscript)
		}
		select {
		case transcriptChan <- finalTranscript:
		default:
		}
		slog.Debug("transcript updated (translated)", "id", transcriptID, "translated", translated)
	}()
}

// Close releases resources.
func (s *Service) Close() error {
	s.Stop()
	return nil
}
