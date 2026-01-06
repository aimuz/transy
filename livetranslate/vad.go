package livetranslate

import (
	"math"
	"time"
)

// VAD (Voice Activity Detector) detects speech in audio streams.
type VAD struct {
	// Thresholds
	threshold float32 // RMS threshold for speech detection

	// Duration constraints
	minSpeechDur    time.Duration // Minimum speech duration before detection
	maxSpeechDur    time.Duration // Maximum speech duration before forced detection
	silenceDur      time.Duration // Silence duration to end speech
	transcribeDelay time.Duration // Minimum delay between detections

	// State
	inSpeech       bool
	speechStart    time.Time
	lastSpeech     time.Time
	lastTranscribe time.Time
}

// NewVAD creates a new voice activity detector with given thresholds.
func NewVAD(threshold float32, minSpeech, maxSpeech, silence, delay time.Duration) *VAD {
	return &VAD{
		threshold:       threshold,
		minSpeechDur:    minSpeech,
		maxSpeechDur:    maxSpeech,
		silenceDur:      silence,
		transcribeDelay: delay,
	}
}

// SpeechEvent represents a detected speech event.
type SpeechEvent struct {
	Type      EventType
	Timestamp time.Time
	// Duration is only populated for SpeechEnd events
	Duration time.Duration
}

// EventType represents the type of speech event.
type EventType int

const (
	EventNone EventType = iota // No event
	EventSpeechStart
	EventSpeechContinue
	EventSpeechEnd
	EventSpeechMaxDuration // Speech exceeded maxSpeechDur
)

// VADResult contains the result of processing audio samples.
type VADResult struct {
	Event            SpeechEvent
	ShouldTranscribe bool // Whether transcription should be triggered
}

// Process processes audio samples and returns detected speech events.
// sampleRate is required to calculate durations (samples per second).
func (v *VAD) Process(samples []float32, sampleRate int) VADResult {
	now := time.Now()

	// Calculate RMS for voice activity detection
	rms := calculateRMS(samples)
	isSpeech := rms > v.threshold

	result := VADResult{
		Event: SpeechEvent{
			Timestamp: now,
			Type:      EventNone,
		},
		ShouldTranscribe: false,
	}

	// Update speech state
	if isSpeech {
		if !v.inSpeech {
			// Speech started
			v.inSpeech = true
			v.speechStart = now
			result.Event.Type = EventSpeechStart
		} else {
			result.Event.Type = EventSpeechContinue
		}
		v.lastSpeech = now
	}

	// Check if we should trigger transcription
	if !v.inSpeech {
		return result
	}

	speechDuration := now.Sub(v.speechStart)
	silenceDuration := now.Sub(v.lastSpeech)

	// Determine if transcription should be triggered:
	// 1. Speech ended (silence detected) and minimum duration met
	// 2. Maximum duration reached
	var shouldTranscribe bool
	var eventType EventType

	if silenceDuration > v.silenceDur && speechDuration > v.minSpeechDur {
		shouldTranscribe = true
		eventType = EventSpeechEnd
		v.inSpeech = false
		result.Event.Duration = speechDuration
	} else if speechDuration > v.maxSpeechDur {
		shouldTranscribe = true
		eventType = EventSpeechMaxDuration
		result.Event.Duration = speechDuration
		// Keep inSpeech = true for continuous long speech
	}

	// Apply rate limiting
	if shouldTranscribe && now.Sub(v.lastTranscribe) < v.transcribeDelay {
		shouldTranscribe = false
		eventType = EventNone
	}

	if shouldTranscribe {
		v.lastTranscribe = now
		result.ShouldTranscribe = true
		result.Event.Type = eventType
	}

	return result
}

// Reset resets the VAD state. Useful when restarting or clearing state.
func (v *VAD) Reset() {
	v.inSpeech = false
	v.speechStart = time.Time{}
	v.lastSpeech = time.Time{}
	v.lastTranscribe = time.Time{}
}

// InSpeech returns true if currently in a speech segment.
func (v *VAD) InSpeech() bool {
	return v.inSpeech
}

// calculateRMS calculates the root mean square of audio samples.
func calculateRMS(samples []float32) float32 {
	if len(samples) == 0 {
		return 0
	}

	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return float32(math.Sqrt(sum / float64(len(samples))))
}
