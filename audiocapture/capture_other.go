// Package audiocapture provides system audio capture.
//
//go:build !darwin

package audiocapture

import "errors"

// ErrNotSupported is returned on unsupported platforms.
var ErrNotSupported = errors.New("audio capture not supported on this platform")

// stubCaptureImpl is a stub implementation for non-macOS platforms.
type stubCaptureImpl struct{}

func newCaptureImpl() (captureImpl, error) {
	return nil, ErrNotSupported
}

func (s *stubCaptureImpl) start(sampleRate int, callback func([]float32)) error {
	return ErrNotSupported
}

func (s *stubCaptureImpl) stop() error {
	return ErrNotSupported
}

func (s *stubCaptureImpl) isCapturing() bool {
	return false
}
