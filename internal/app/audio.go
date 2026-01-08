package app

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.aimuz.me/transy/audiocapture"
)

// AudioAdapter manages audio capture with proper synchronization.
type AudioAdapter struct {
	mu       sync.Mutex
	capture  audiocapture.Capturer
	stopChan chan struct{}
}

// Start begins audio capture and streams samples via the emit function.
func (aa *AudioAdapter) Start(emit func(name string, data any)) error {
	aa.mu.Lock()
	defer aa.mu.Unlock()

	if aa.capture != nil {
		return fmt.Errorf("audio capture already running")
	}

	// Create audio capture at 48kHz (WebRTC Opus standard)
	cap, err := audiocapture.New(48000)
	if err != nil {
		return fmt.Errorf("create audio capture: %w", err)
	}

	aa.stopChan = make(chan struct{})
	seq := 0

	// Start capturing
	if err := cap.Start(func(samples []float32) {
		select {
		case <-aa.stopChan:
			return
		default:
		}

		seq++
		emit(EventAudioSamples, AudioSamples{
			Samples:   samples,
			Timestamp: time.Now().UnixMilli(),
			Seq:       seq,
		})

		if seq%100 == 0 {
			slog.Debug("streamed audio samples", "count", seq, "samples", len(samples))
		}
	}); err != nil {
		return fmt.Errorf("start audio capture: %w", err)
	}

	aa.capture = cap
	slog.Info("audio capture started")
	return nil
}

// Stop stops audio capture.
func (aa *AudioAdapter) Stop() error {
	aa.mu.Lock()
	defer aa.mu.Unlock()

	if aa.capture == nil {
		return nil
	}

	if aa.stopChan != nil {
		close(aa.stopChan)
		aa.stopChan = nil
	}

	err := aa.capture.Stop()
	aa.capture = nil

	slog.Info("audio capture stopped")
	return err
}
