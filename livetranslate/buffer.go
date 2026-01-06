package livetranslate

import "time"

// AudioBuffer manages audio sample accumulation with overlap support.
// It provides a simple interface for buffering audio data.
type AudioBuffer struct {
	samples      []float32
	overlapRatio float64 // Ratio of samples to keep on clear (0-1)
	sampleRate   int
}

// NewAudioBuffer creates a new audio buffer.
// overlapRatio determines how much of the buffer to retain after extraction (e.g., 0.5 = 50% overlap).
func NewAudioBuffer(sampleRate int, overlapRatio float64) *AudioBuffer {
	return &AudioBuffer{
		samples:      make([]float32, 0, sampleRate*30), // 30 second capacity
		overlapRatio: overlapRatio,
		sampleRate:   sampleRate,
	}
}

// Append adds new audio samples to the buffer.
func (b *AudioBuffer) Append(samples []float32) {
	b.samples = append(b.samples, samples...)
}

// Extract returns a copy of all buffered samples and clears the buffer,
// retaining an overlap based on overlapRatio for continuity.
func (b *AudioBuffer) Extract() []float32 {
	if len(b.samples) == 0 {
		return nil
	}

	// Make a copy
	result := make([]float32, len(b.samples))
	copy(result, b.samples)

	// Keep overlap
	overlapSamples := int(float64(len(b.samples)) * b.overlapRatio)
	if overlapSamples > 0 && overlapSamples < len(b.samples) {
		copy(b.samples, b.samples[len(b.samples)-overlapSamples:])
		b.samples = b.samples[:overlapSamples]
	} else {
		b.samples = b.samples[:0]
	}

	return result
}

// ExtractWithDuration is like Extract but also calculates the segment timing.
// Returns samples, segment start time relative to sessionStart, and duration.
func (b *AudioBuffer) ExtractWithDuration(sessionStart time.Time) ([]float32, int64, int64) {
	samples := b.Extract()
	if len(samples) == 0 {
		return nil, 0, 0
	}

	duration := int64(float64(len(samples)) / float64(b.sampleRate) * 1000) // ms
	endTime := time.Since(sessionStart).Milliseconds()
	startTime := endTime - duration

	return samples, startTime, duration
}

// Clear empties the buffer completely.
func (b *AudioBuffer) Clear() {
	b.samples = b.samples[:0]
}

// Len returns the number of samples currently in the buffer.
func (b *AudioBuffer) Len() int {
	return len(b.samples)
}

// Duration returns the duration of buffered audio in milliseconds.
func (b *AudioBuffer) Duration() int64 {
	if len(b.samples) == 0 || b.sampleRate == 0 {
		return 0
	}
	return int64(float64(len(b.samples)) / float64(b.sampleRate) * 1000)
}
