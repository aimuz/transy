package livetranslate

import (
	"testing"
	"time"
)

// TestVAD_SpeechDetection tests basic speech detection functionality
func TestVAD_SpeechDetection(t *testing.T) {
	tests := []struct {
		name           string
		samples        []float32
		wantEventType  EventType
		wantTranscribe bool
		wantInSpeech   bool
	}{
		{
			name:           "silence - no speech",
			samples:        makeSilence(1000),
			wantEventType:  EventNone,
			wantTranscribe: false,
			wantInSpeech:   false,
		},
		{
			name:           "speech start - loud audio",
			samples:        makeSpeech(1000, 0.05),
			wantEventType:  EventSpeechStart,
			wantTranscribe: false,
			wantInSpeech:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewVAD(
				0.02,                 // threshold
				300*time.Millisecond, // minSpeech
				5*time.Second,        // maxSpeech
				400*time.Millisecond, // silence
				300*time.Millisecond, // delay
			)

			result := v.Process(tt.samples, 16000)

			if result.Event.Type != tt.wantEventType {
				t.Errorf("Event.Type = %v, want %v", result.Event.Type, tt.wantEventType)
			}
			if result.ShouldTranscribe != tt.wantTranscribe {
				t.Errorf("ShouldTranscribe = %v, want %v", result.ShouldTranscribe, tt.wantTranscribe)
			}
			if v.InSpeech() != tt.wantInSpeech {
				t.Errorf("InSpeech() = %v, want %v", v.InSpeech(), tt.wantInSpeech)
			}
		})
	}
}

// TestVAD_SpeechSequence tests a realistic sequence of speech events
func TestVAD_SpeechSequence(t *testing.T) {
	v := NewVAD(
		0.02,                 // threshold
		300*time.Millisecond, // minSpeech
		5*time.Second,        // maxSpeech
		400*time.Millisecond, // silence
		300*time.Millisecond, // delay
	)

	// Sequence of events that simulate real speech
	sequence := []struct {
		name           string
		samples        []float32
		sleep          time.Duration
		wantEventType  EventType
		wantTranscribe bool
	}{
		{
			name:           "1. start with silence",
			samples:        makeSilence(1000),
			wantEventType:  EventNone,
			wantTranscribe: false,
		},
		{
			name:           "2. speech starts",
			samples:        makeSpeech(1000, 0.05),
			wantEventType:  EventSpeechStart,
			wantTranscribe: false,
		},
		{
			name:           "3. speech continues",
			samples:        makeSpeech(1000, 0.04),
			sleep:          100 * time.Millisecond,
			wantEventType:  EventSpeechContinue,
			wantTranscribe: false,
		},
		{
			name:           "4. more speech",
			samples:        makeSpeech(1000, 0.03),
			sleep:          200 * time.Millisecond,
			wantEventType:  EventSpeechContinue,
			wantTranscribe: false,
		},
		{
			name:           "5. silence - trigger transcription",
			samples:        makeSilence(1000),
			sleep:          500 * time.Millisecond,
			wantEventType:  EventSpeechEnd,
			wantTranscribe: true,
		},
		{
			name:           "6. more silence - no transcription (rate limited)",
			samples:        makeSilence(1000),
			wantEventType:  EventNone,
			wantTranscribe: false,
		},
	}

	for _, step := range sequence {
		t.Run(step.name, func(t *testing.T) {
			if step.sleep > 0 {
				time.Sleep(step.sleep)
			}

			result := v.Process(step.samples, 16000)

			if result.Event.Type != step.wantEventType {
				t.Errorf("Event.Type = %v, want %v", result.Event.Type, step.wantEventType)
			}
			if result.ShouldTranscribe != step.wantTranscribe {
				t.Errorf("ShouldTranscribe = %v, want %v", result.ShouldTranscribe, step.wantTranscribe)
			}
		})
	}
}

// TestVAD_MaxDuration tests that long speech triggers transcription
func TestVAD_MaxDuration(t *testing.T) {
	v := NewVAD(
		0.02,
		300*time.Millisecond,
		1*time.Second, // Very short max duration for testing
		400*time.Millisecond,
		300*time.Millisecond,
	)

	// Start speech
	v.Process(makeSpeech(1000, 0.05), 16000)

	// Continue speech for a bit
	time.Sleep(500 * time.Millisecond)
	v.Process(makeSpeech(1000, 0.05), 16000)

	// Continue speech past max duration
	time.Sleep(600 * time.Millisecond)
	result := v.Process(makeSpeech(1000, 0.05), 16000)

	if result.Event.Type != EventSpeechMaxDuration {
		t.Errorf("Event.Type = %v, want EventSpeechMaxDuration", result.Event.Type)
	}
	if !result.ShouldTranscribe {
		t.Error("ShouldTranscribe = false, want true for max duration")
	}
	if result.Event.Duration < 1*time.Second {
		t.Errorf("Event.Duration = %v, want >= 1s", result.Event.Duration)
	}
}

// TestVAD_Reset tests that Reset clears all state
func TestVAD_Reset(t *testing.T) {
	v := NewVAD(0.02, 300*time.Millisecond, 5*time.Second, 400*time.Millisecond, 300*time.Millisecond)

	// Trigger speech
	v.Process(makeSpeech(1000, 0.05), 16000)

	if !v.InSpeech() {
		t.Fatal("Expected InSpeech() = true before reset")
	}

	// Reset
	v.Reset()

	if v.InSpeech() {
		t.Error("Expected InSpeech() = false after reset")
	}

	// Should be able to detect speech again
	result := v.Process(makeSpeech(1000, 0.05), 16000)
	if result.Event.Type != EventSpeechStart {
		t.Errorf("After reset, got Event.Type = %v, want EventSpeechStart", result.Event.Type)
	}
}

// TestCalculateRMS tests RMS calculation
func TestCalculateRMS(t *testing.T) {
	tests := []struct {
		name    string
		samples []float32
		want    float32
	}{
		{
			name:    "empty samples",
			samples: []float32{},
			want:    0,
		},
		{
			name:    "all zeros",
			samples: []float32{0, 0, 0, 0},
			want:    0,
		},
		{
			name:    "simple positive values",
			samples: []float32{0.1, 0.1, 0.1, 0.1},
			want:    0.1,
		},
		{
			name:    "mixed positive/negative",
			samples: []float32{0.3, -0.3, 0.3, -0.3},
			want:    0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRMS(tt.samples)
			// Allow small floating point error
			if abs(got-tt.want) > 0.001 {
				t.Errorf("calculateRMS() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper functions for generating test audio

func makeSilence(samples int) []float32 {
	return make([]float32, samples)
}

func makeSpeech(samples int, amplitude float32) []float32 {
	result := make([]float32, samples)
	for i := range result {
		// Simple sine wave to simulate speech
		result[i] = amplitude * float32(0.5+0.5*float64(i%2))
	}
	return result
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
