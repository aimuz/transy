// Package audiocapture provides system audio capture.
//
// On macOS, it uses ScreenCaptureKit to capture system audio.
// Other platforms return ErrUnsupported.
package audiocapture

import "errors"

// Sentinel errors.
var (
	ErrUnsupported = errors.New("audiocapture: unsupported platform")
	ErrRunning     = errors.New("audiocapture: already running")
	ErrStopped     = errors.New("audiocapture: not running")
)

// AudioHandler processes captured audio samples.
// Samples are float32 in range [-1, 1] at the configured sample rate.
// The handler is called from a platform-specific audio thread;
// implementations should avoid blocking.
type AudioHandler func(samples []float32)

// Capturer captures system audio.
type Capturer interface {
	// Start begins audio capture. The handler receives audio samples
	// until Stop is called. Returns ErrRunning if already capturing.
	Start(handler AudioHandler) error

	// Stop ends audio capture. Safe to call if not running.
	Stop() error
}
