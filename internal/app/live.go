package app

import (
	"context"
	"log/slog"
	"sync"

	"go.aimuz.me/transy/internal/types"
)

// LiveAdapter manages live translation with proper synchronization.
type LiveAdapter struct {
	mu      sync.RWMutex
	service types.LiveTranslator
	cancel  context.CancelFunc
}

// Start begins live translation. Stops any existing session first.
func (la *LiveAdapter) Start(ctx context.Context, service types.LiveTranslator, sourceLang, targetLang string) error {
	la.mu.Lock()
	defer la.mu.Unlock()

	// Stop existing service if running
	if la.service != nil {
		_ = la.service.Stop()
		la.service = nil
	}
	if la.cancel != nil {
		la.cancel()
		la.cancel = nil
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	la.cancel = cancel

	if err := service.Start(ctx, sourceLang, targetLang); err != nil {
		cancel()
		return err
	}

	la.service = service
	return nil
}

// Stop stops the live translation service.
func (la *LiveAdapter) Stop() error {
	la.mu.Lock()
	defer la.mu.Unlock()

	if la.cancel != nil {
		la.cancel()
		la.cancel = nil
	}

	if la.service == nil {
		return nil
	}

	err := la.service.Stop()
	la.service = nil
	return err
}

// Status returns the current status, safe for concurrent access.
func (la *LiveAdapter) Status() types.LiveStatus {
	la.mu.RLock()
	defer la.mu.RUnlock()

	if la.service == nil {
		return types.LiveStatus{}
	}
	return la.service.Status()
}

// ForwardEvents forwards all events from the service to the emitter.
// Blocks until the service is stopped. Should be called in a goroutine.
func (la *LiveAdapter) ForwardEvents(emit func(name string, data any), translate func(t types.LiveTranscript)) {
	la.mu.RLock()
	svc := la.service
	la.mu.RUnlock()

	if svc == nil {
		return
	}

	var wg sync.WaitGroup

	// Forward transcripts
	wg.Go(func() {
		for transcript := range svc.Transcripts() {
			emit(EventLiveTranscript, transcript)

			// Async translate if final with source text but no target text
			if transcript.IsFinal && transcript.SourceText != "" && transcript.TargetText == "" {
				go translate(transcript)
			}
		}
	})

	// Forward VAD updates
	wg.Go(func() {
		defer wg.Done()
		for state := range svc.VADUpdates() {
			emit(EventVADUpdate, state)
		}
	})

	// Log errors
	wg.Go(func() {
		defer wg.Done()
		for err := range svc.Errors() {
			slog.Error("live translation error", "error", err)
		}
	})
	wg.Wait()
}
