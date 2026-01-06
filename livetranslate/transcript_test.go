package livetranslate

import (
	"testing"
	"time"
)

func TestTranscriptMerger_SingleSegment(t *testing.T) {
	m := NewTranscriptMerger(1*time.Second, 5)

	result := m.Process("Hello world", time.Now())

	if result.IsMerged {
		t.Error("First segment should not be merged")
	}
	if !result.IsNewSegment {
		t.Error("First segment should be new")
	}
	if result.FullText != "Hello world" {
		t.Errorf("FullText = %q, want %q", result.FullText, "Hello world")
	}
	if result.ID == "" {
		t.Error("ID should not be empty")
	}
	if len(result.Context) != 0 {
		t.Errorf("Context should be empty for first segment, got %v", result.Context)
	}
}

func TestTranscriptMerger_MergeSequence(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		delay        time.Duration
		wantMerged   bool
		wantFullText string
		wantContext  int
	}{
		{
			name:         "1. first segment",
			text:         "Hello",
			delay:        0,
			wantMerged:   false,
			wantFullText: "Hello",
			wantContext:  0,
		},
		{
			name:         "2. quick follow-up - should merge",
			text:         "world",
			delay:        500 * time.Millisecond,
			wantMerged:   true,
			wantFullText: "Hello world",
			wantContext:  0, // No context excluding self
		},
		{
			name:         "3. another quick segment - merge again",
			text:         "today",
			delay:        500 * time.Millisecond,
			wantMerged:   true,
			wantFullText: "Hello world today",
			wantContext:  0,
		},
		{
			name:         "4. long pause - new segment",
			text:         "New sentence",
			delay:        1500 * time.Millisecond,
			wantMerged:   false,
			wantFullText: "New sentence",
			wantContext:  1, // Previous merged segment
		},
		{
			name:         "5. quick follow - merge with new",
			text:         "here",
			delay:        500 * time.Millisecond,
			wantMerged:   true,
			wantFullText: "New sentence here",
			wantContext:  1, // Previous segment
		},
	}

	m := NewTranscriptMerger(1*time.Second, 5)
	now := time.Now()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now = now.Add(tt.delay)
			result := m.Process(tt.text, now)

			if result.IsMerged != tt.wantMerged {
				t.Errorf("IsMerged = %v, want %v", result.IsMerged, tt.wantMerged)
			}
			if result.FullText != tt.wantFullText {
				t.Errorf("FullText = %q, want %q", result.FullText, tt.wantFullText)
			}
			if len(result.Context) != tt.wantContext {
				t.Errorf("Context length = %d, want %d", len(result.Context), tt.wantContext)
			}
		})
	}
}

func TestTranscriptMerger_ContextManagement(t *testing.T) {
	// Use small maxContext for testing
	m := NewTranscriptMerger(2*time.Second, 3)
	now := time.Now()

	segments := []string{"One", "Two", "Three", "Four", "Five"}

	for i, text := range segments {
		// Add delays to prevent merging
		now = now.Add(3 * time.Second)
		result := m.Process(text, now)

		// Check context size doesn't exceed maxContext
		recentContext := m.GetRecentContext()
		expectedSize := i + 1
		if expectedSize > 3 {
			expectedSize = 3
		}
		if len(recentContext) != expectedSize {
			t.Errorf("After segment %d, context size = %d, want %d",
				i+1, len(recentContext), expectedSize)
		}

		// Verify most recent segments are kept
		if i >= 3 {
			// Should have segments (i-2), (i-1), i
			expectedTexts := segments[i-2 : i+1]
			for j, expected := range expectedTexts {
				if recentContext[j] != expected {
					t.Errorf("Context[%d] = %q, want %q", j, recentContext[j], expected)
				}
			}
		}

		// Context in result should exclude current segment
		if result.IsMerged {
			// If merged, current text is modification of last context entry
			// So context should have one less
			continue
		}
		expectedCtxSize := expectedSize - 1
		if len(result.Context) != expectedCtxSize {
			t.Errorf("Result context size = %d, want %d", len(result.Context), expectedCtxSize)
		}
	}
}

func TestTranscriptMerger_IDGeneration(t *testing.T) {
	m := NewTranscriptMerger(1*time.Second, 5)
	now := time.Now()

	// First segment
	r1 := m.Process("First", now)
	id1 := r1.ID

	// Quick follow-up - should merge (same ID)
	now = now.Add(500 * time.Millisecond)
	r2 := m.Process("Second", now)

	if r2.ID != id1 {
		t.Errorf("Merged segment ID = %q, want %q (same as previous)", r2.ID, id1)
	}

	// Long pause - new segment (new ID)
	now = now.Add(2 * time.Second)
	r3 := m.Process("Third", now)

	if r3.ID == id1 {
		t.Error("New segment should have different ID")
	}
	if r3.ID == "" {
		t.Error("New segment ID should not be empty")
	}
}

func TestTranscriptMerger_Reset(t *testing.T) {
	m := NewTranscriptMerger(1*time.Second, 5)
	now := time.Now()

	// Create some segments
	m.Process("First", now)
	now = now.Add(500 * time.Millisecond)
	m.Process("Second", now)

	if m.Count() != 1 {
		t.Errorf("Count before reset = %d, want 1", m.Count())
	}

	// Reset
	m.Reset()

	if m.Count() != 0 {
		t.Errorf("Count after reset = %d, want 0", m.Count())
	}

	context := m.GetRecentContext()
	if len(context) != 0 {
		t.Errorf("Context after reset should be empty, got %v", context)
	}

	// Should work normally after reset
	result := m.Process("After reset", time.Now())
	if result.IsMerged {
		t.Error("First segment after reset should not be merged")
	}
	if result.ID == "" {
		t.Error("ID should not be empty after reset")
	}
}

func TestTranscriptMerger_MultipleMerges(t *testing.T) {
	m := NewTranscriptMerger(1*time.Second, 5)
	now := time.Now()

	// Create a segment and merge 5 times
	texts := []string{"One", "Two", "Three", "Four", "Five", "Six"}
	expectedFull := ""

	for i, text := range texts {
		if i > 0 {
			expectedFull += " "
		}
		expectedFull += text

		result := m.Process(text, now)
		now = now.Add(500 * time.Millisecond) // Within merge threshold

		if i == 0 {
			if result.IsMerged {
				t.Error("First segment should not be merged")
			}
		} else {
			if !result.IsMerged {
				t.Errorf("Segment %d should be merged", i+1)
			}
		}

		if result.FullText != expectedFull {
			t.Errorf("After segment %d, FullText = %q, want %q",
				i+1, result.FullText, expectedFull)
		}
	}

	// Should still have only 1 segment (all merged)
	if m.Count() != 1 {
		t.Errorf("Count = %d, want 1 (all segments merged)", m.Count())
	}
}
