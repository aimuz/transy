package audiocapture

import (
	"errors"
	"runtime"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate int
		wantErr    error
	}{
		{"whisper_16k", 16000, nil},
		{"webrtc_48k", 48000, nil},
		{"zero_defaults", 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.sampleRate)

			// Platform-dependent behavior
			if runtime.GOOS != "darwin" {
				if !errors.Is(err, ErrUnsupported) {
					t.Fatalf("expected ErrUnsupported on %s, got %v", runtime.GOOS, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c == nil {
				t.Fatal("expected non-nil Capturer")
			}
		})
	}
}

func TestStartWithNilHandler(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("skipping on non-darwin")
	}

	c, err := New(16000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.Start(nil); err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestDoubleStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("skipping on non-darwin")
	}

	c, err := New(16000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Stop()

	// First start should succeed
	if err := c.Start(func([]float32) {}); err != nil {
		t.Fatalf("first Start: %v", err)
	}

	// Second start should fail
	if err := c.Start(func([]float32) {}); !errors.Is(err, ErrRunning) {
		t.Fatalf("expected ErrRunning, got %v", err)
	}
}

func TestStopIdempotent(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("skipping on non-darwin")
	}

	c, err := New(16000)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Stop without start should be safe
	if err := c.Stop(); err != nil {
		t.Fatalf("Stop without Start: %v", err)
	}

	// Double stop should be safe
	if err := c.Stop(); err != nil {
		t.Fatalf("double Stop: %v", err)
	}
}
