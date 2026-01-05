// Package audiocapture provides system audio capture using ScreenCaptureKit.
package audiocapture

import (
	"errors"
	"sync"
	"time"
)

// ErrNotCapturing is returned when trying to get audio while not capturing.
var ErrNotCapturing = errors.New("not capturing audio")

// ErrAlreadyCapturing is returned when trying to start capture while already capturing.
var ErrAlreadyCapturing = errors.New("already capturing audio")

// Capture provides system audio capture functionality.
// It uses ScreenCaptureKit on macOS to capture system audio without
// requiring a virtual audio device like BlackHole.
type Capture struct {
	mu sync.RWMutex

	// State
	capturing  bool
	startTime  time.Time
	sampleRate int

	// Audio buffer
	buffer     *RingBuffer
	bufferSize int // in samples

	// Callbacks
	onAudio []func(samples []float32)

	// Platform-specific implementation
	impl captureImpl
}

// captureImpl is the platform-specific capture implementation interface.
type captureImpl interface {
	start(sampleRate int, callback func(samples []float32)) error
	stop() error
	isCapturing() bool
}

// Config holds configuration for audio capture.
type Config struct {
	SampleRate int           // Sample rate, default 16000 Hz (optimal for Whisper)
	BufferSize time.Duration // Buffer duration, default 30 seconds
}

// DefaultConfig returns the default capture configuration.
func DefaultConfig() Config {
	return Config{
		SampleRate: 16000, // Whisper expects 16kHz
		BufferSize: 30 * time.Second,
	}
}

// New creates a new audio capture instance.
func New(cfg Config) (*Capture, error) {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 30 * time.Second
	}

	bufferSamples := int(cfg.BufferSize.Seconds()) * cfg.SampleRate

	c := &Capture{
		sampleRate: cfg.SampleRate,
		bufferSize: bufferSamples,
		buffer:     NewRingBuffer(bufferSamples),
		onAudio:    make([]func(samples []float32), 0),
	}

	// Create platform-specific implementation
	impl, err := newCaptureImpl()
	if err != nil {
		return nil, err
	}
	c.impl = impl

	return c, nil
}

// Start begins capturing system audio.
func (c *Capture) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.capturing {
		return ErrAlreadyCapturing
	}

	err := c.impl.start(c.sampleRate, func(samples []float32) {
		c.handleAudio(samples)
	})
	if err != nil {
		return err
	}

	c.capturing = true
	c.startTime = time.Now()
	return nil
}

// Stop stops capturing audio.
func (c *Capture) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.capturing {
		return nil
	}

	err := c.impl.stop()
	c.capturing = false
	return err
}

// IsCapturing returns true if currently capturing audio.
func (c *Capture) IsCapturing() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capturing
}

// Duration returns how long capture has been running.
func (c *Capture) Duration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.capturing {
		return 0
	}
	return time.Since(c.startTime)
}

// OnAudio registers a callback for audio data.
// The callback receives float32 samples in the range [-1, 1].
func (c *Capture) OnAudio(callback func(samples []float32)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onAudio = append(c.onAudio, callback)
}

// GetBufferedAudio returns the last 'duration' of buffered audio.
// This is useful for getting context when starting transcription.
func (c *Capture) GetBufferedAudio(duration time.Duration) []float32 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	samples := int(duration.Seconds() * float64(c.sampleRate))
	return c.buffer.Read(samples)
}

// handleAudio processes incoming audio samples.
func (c *Capture) handleAudio(samples []float32) {
	c.mu.RLock()
	callbacks := c.onAudio
	c.mu.RUnlock()

	// Store in buffer
	c.buffer.Write(samples)

	// Notify callbacks
	for _, cb := range callbacks {
		cb(samples)
	}
}

// SampleRate returns the configured sample rate.
func (c *Capture) SampleRate() int {
	return c.sampleRate
}

// RingBuffer is a thread-safe circular buffer for audio samples.
type RingBuffer struct {
	mu       sync.RWMutex
	data     []float32
	writePos int
	size     int
	filled   int // How many samples have been written (up to size)
}

// NewRingBuffer creates a new ring buffer with the given capacity.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data: make([]float32, size),
		size: size,
	}
}

// Write adds samples to the buffer.
func (rb *RingBuffer) Write(samples []float32) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, s := range samples {
		rb.data[rb.writePos] = s
		rb.writePos = (rb.writePos + 1) % rb.size
		if rb.filled < rb.size {
			rb.filled++
		}
	}
}

// Read returns the last n samples from the buffer.
func (rb *RingBuffer) Read(n int) []float32 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n > rb.filled {
		n = rb.filled
	}
	if n == 0 {
		return nil
	}

	result := make([]float32, n)

	// Calculate start position
	startPos := (rb.writePos - n + rb.size) % rb.size

	// Copy samples
	for i := 0; i < n; i++ {
		result[i] = rb.data[(startPos+i)%rb.size]
	}

	return result
}

// Clear empties the buffer.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.writePos = 0
	rb.filled = 0
}

// Len returns the number of samples in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.filled
}
