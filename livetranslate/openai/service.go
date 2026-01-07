package openai

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.aimuz.me/transy/audiocapture"
	"go.aimuz.me/transy/internal/types"
)

// ServiceConfig holds configuration for the Realtime Service.
// Immutable once created.
type ServiceConfig struct {
	APIKey       string
	Model        string
	SystemPrompt string
	Temperature  float64
}

// sessionState holds mutable state for a single running session.
// Designed for copy-on-write pattern.
// sessionState holds mutable state for a single running session.
// Designed for copy-on-write pattern.
type sessionState struct {
	sourceLang string
	targetLang string
	startTime  time.Time
	count      int

	// VAD State
	segmentID string // Current active segment ID (for VAD display)
	vadState  types.VADState
}

// itemState tracks the content of a single speech item.
type itemState struct {
	ID          string
	SourceText  string
	TargetText  string
	StartTime   int64
	EndTime     int64 // Set when SpeechStopped
	SourceFinal bool
	TargetFinal bool
}

// Service provides real-time speech-to-speech/text execution using OpenAI Realtime API.
type Service struct {
	// Configuration (immutable after creation)
	config ServiceConfig

	// Dependencies
	client *Client
	audio  audiocapture.Capturer

	// State - atomic for lock-free reads
	running atomic.Bool
	sess    atomic.Pointer[sessionState]

	// Initialization lock (only for Start/Stop)
	mu     sync.Mutex
	cancel context.CancelFunc

	// Output channels
	transcriptChan chan types.LiveTranscript
	vadChan        chan types.VADState
	errorChan      chan error

	// Item State - Mutex protected for concurrent updates
	muItems     sync.Mutex
	activeItems map[string]*itemState // Map[ItemID]*itemState
}

// NewService creates a new Realtime Service.
func NewService(cfg ServiceConfig) (*Service, error) {
	// WebRTC Opus uses 48kHz - capture at native rate
	audioCap, err := audiocapture.New(48000)
	if err != nil {
		return nil, fmt.Errorf("create audio capture: %w", err)
	}

	return &Service{
		config: cfg,
		audio:  audioCap,
	}, nil
}

// Start begins the realtime session.
func (s *Service) Start(ctx context.Context, sourceLang, targetLang string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running.Load() {
		return fmt.Errorf("already running")
	}

	// Initialize session state
	sess := &sessionState{
		sourceLang: sourceLang,
		targetLang: targetLang,
		startTime:  time.Now(),
	}
	s.sess.Store(sess)

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Initialize channels
	s.transcriptChan = make(chan types.LiveTranscript, 100)
	s.vadChan = make(chan types.VADState, 100)
	s.errorChan = make(chan error, 10)

	// Initialize state maps
	s.activeItems = make(map[string]*itemState)

	// Create client
	client, err := NewClient(Config{
		APIKey: s.config.APIKey,
		Session: SessionConfig{
			Model:  s.config.Model,
			Prompt: s.config.SystemPrompt,
		},
	})
	if err != nil {
		cancel()
		return fmt.Errorf("create client: %w", err)
	}
	s.client = client

	// Setup callbacks
	s.client.OnDataChannelOpen(func() {
		slog.Info("data channel ready")
	})

	// Connect
	if err := s.client.Connect(ctx); err != nil {
		cancel()
		return fmt.Errorf("connect client: %w", err)
	}

	// Start Audio with handler
	if err := s.audio.Start(s.handleAudio); err != nil {
		s.client.Close()
		cancel()
		return fmt.Errorf("start audio: %w", err)
	}

	s.running.Store(true)
	go s.processEvents()

	slog.Info("realtime service started")
	return nil
}

// Stop ends the realtime session.
func (s *Service) Stop() error {
	s.mu.Lock()
	if !s.running.Load() {
		s.mu.Unlock()
		return nil
	}
	s.running.Store(false)
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	if s.audio != nil {
		_ = s.audio.Stop()
	}
	if s.client != nil {
		_ = s.client.Close()
	}

	return nil
}

func (s *Service) handleAudio(samples []float32) {
	if err := s.client.SendAudio(samples); err != nil {
		slog.Warn("failed to send audio", "error", err)
	}
}

func (s *Service) processEvents() {
	defer func() {
		close(s.transcriptChan)
		close(s.vadChan)
		close(s.errorChan)
	}()

	for event := range s.client.Messages() {
		switch e := event.(type) {
		case TranscriptEvent:
			s.handleTranscript(e)
		case TranscriptDeltaEvent:
			s.handleTranscriptDelta(e)
		case SpeechStartedEvent:
			s.handleSpeechStarted(e)
		case SpeechStoppedEvent:
			s.handleSpeechStopped(e)
		case ItemDoneEvent:
			if e.Item.Role == "assistant" {
				s.updateVAD(types.VADStateListening)
			}
		case ErrorEvent:
			s.sendError(fmt.Errorf("api error: %s (%s)", e.Error.Message, e.Error.Code))
		}
	}
}

// handleSpeechStarted handles VAD speech start event.
// It initializes a new segment immediately with the ItemID.
// handleSpeechStarted handles VAD speech start event.
// It initializes a new segment immediately with the ItemID.
func (s *Service) handleSpeechStarted(e SpeechStartedEvent) {
	s.updateVAD(types.VADStateSpeaking)

	sess := s.sess.Load()
	if sess == nil {
		return
	}

	// Lock item state to register new item
	s.muItems.Lock()

	// Initialize new item state
	newItem := &itemState{
		ID:        e.ItemID,
		StartTime: time.Since(sess.startTime).Milliseconds(),
	}
	s.activeItems[e.ItemID] = newItem
	s.muItems.Unlock()

	// 在使用语义化的 vad 时，这个将会频繁触发，所以我们忽略这个事件，仅作为记录
	// s.emit(newItem, sess)

	for {
		currentSess := s.sess.Load()
		if currentSess == nil {
			return
		}

		// If there was an active previous segment, we might want to ensure it's finalized?
		// Actually, the previous item's handlers will finalize it when they receive events.
		// We just update the POINTER to the current segment ID for VAD purposes.

		updated := *currentSess
		updated.segmentID = e.ItemID
		// Removed fields: segmentStart, sourceText, targetText, responseID are GONE from sessionState.

		if s.sess.CompareAndSwap(currentSess, &updated) {
			return
		}
	}
}

// handleSpeechStopped handles VAD speech stop event.
func (s *Service) handleSpeechStopped(e SpeechStoppedEvent) {
	s.updateVAD(types.VADStateProcessing)

	s.muItems.Lock()
	defer s.muItems.Unlock()

	item, ok := s.activeItems[e.ItemID]
	if !ok {
		return
	}

	// Calculate approximate duration based on local clock, or just set EndTime relative to session start.
	// We want EndTime to be consistent with StartTime.
	// Since StartTime = time.Since(sess.startTime), EndTime should be same base.

	// We need sess for startTime base.
	sess := s.sess.Load()
	if sess != nil {
		item.EndTime = time.Since(sess.startTime).Milliseconds()
	}

	s.emit(item, sess)
}

func (s *Service) handleTranscript(e TranscriptEvent) {
	slog.Debug("transcript completed", "text", e.Transcript)

	s.muItems.Lock()
	defer s.muItems.Unlock()

	item, ok := s.activeItems[e.ItemID]
	if !ok {
		return
	}

	item.SourceText = e.Transcript
	item.SourceFinal = true

	// OpenAI guarantees this event comes after speech stopped and audio is processed.
	s.emit(item, s.sess.Load())
}

func (s *Service) handleTranscriptDelta(e TranscriptDeltaEvent) {
	s.muItems.Lock()
	defer s.muItems.Unlock()

	item, ok := s.activeItems[e.ItemID]
	if !ok {
		return
	}

	item.SourceText += e.Delta
	s.emit(item, s.sess.Load())
}

func (s *Service) updateVAD(state types.VADState) {
	for {
		sess := s.sess.Load()
		if sess == nil {
			return
		}
		if sess.vadState == state {
			return
		}

		updated := *sess
		updated.vadState = state
		if s.sess.CompareAndSwap(sess, &updated) {
			select {
			case s.vadChan <- state:
			default:
			}
			return
		}
	}
}

func (s *Service) emit(item *itemState, sess *sessionState) {
	if item == nil || sess == nil {
		return
	}

	// Determine finality: For now, if SourceFinal is true, we consider it "Done" enough for display update?
	// Or maybe only when TargetFinal?
	// User said "conversation.item.input_audio_transcription.completed marks completion".
	// Let's assume SourceFinal dictates at least the "Audio/Source" part is done.
	// If we are waiting for translation, we might still be !IsFinal overall.
	// BUT, if we removed TextDelta/Done, maybe we aren't getting translation back?
	// If we are just doing STT, then SourceFinal is enough.
	// If we are doing translation, we might wait.
	// However, user said "Can be ignored" for Text events. Maybe they rely on something else or just don't want AI response text events.
	// Let's rely on SourceFinal for IsFinal if TargetText is empty or not expected.
	// For now, let's keep strict check: IsFinal if we think it's done.
	// If the user removed TextDone, maybe they don't expect TargetText?
	// Or TargetText comes in Transcript? No.
	// Let's set IsFinal = SourceFinal for now as requested by implication.
	isFinal := item.SourceFinal

	// Calc end time if final
	var end int64
	if isFinal {
		end = item.EndTime
		// Fallback if SpeechStopped missed?
		if end == 0 {
			end = time.Since(sess.startTime).Milliseconds()
		}
	} else if item.EndTime > 0 {
		end = item.EndTime
	}

	t := types.LiveTranscript{
		ID:         item.ID,
		SourceText: item.SourceText,
		TargetText: item.TargetText,
		SourceLang: sess.sourceLang,
		TargetLang: sess.targetLang,
		StartTime:  item.StartTime,
		EndTime:    end,
		Timestamp:  time.Now().UnixMilli(),
		IsFinal:    isFinal,
		Confidence: 1.0,
	}

	slog.Debug("emit", "data", t)

	select {
	case s.transcriptChan <- t:
	default:
		// Drop if full to avoid blocking event loop
	}
}

func (s *Service) sendError(err error) {
	select {
	case s.errorChan <- err:
	default:
	}
}

// Transcripts returns a read-only channel for receiving transcripts.
func (s *Service) Transcripts() <-chan types.LiveTranscript {
	return s.transcriptChan
}

func (s *Service) Errors() <-chan error {
	return s.errorChan
}

// VADUpdates returns a read-only channel for receiving VAD state changes.
func (s *Service) VADUpdates() <-chan types.VADState {
	return s.vadChan
}

// Status returns the current status of the translation service.
func (s *Service) Status() types.LiveStatus {
	sess := s.sess.Load()

	var duration int64
	var sourceLang, targetLang string
	var count int

	if sess != nil {
		if s.running.Load() {
			duration = int64(time.Since(sess.startTime).Seconds())
		}
		sourceLang = sess.sourceLang
		targetLang = sess.targetLang
		count = sess.count
	}

	return types.LiveStatus{
		Active:          s.running.Load(),
		SourceLang:      sourceLang,
		TargetLang:      targetLang,
		STTProvider:     "OpenAI Realtime",
		Duration:        duration,
		TranscriptCount: count,
		VADState:        sess.vadState,
	}
}

// Close stops the service.
func (s *Service) Close() error {
	return s.Stop()
}
