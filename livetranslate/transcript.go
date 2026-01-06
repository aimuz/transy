package livetranslate

import (
	"fmt"
	"sync/atomic"
	"time"
)

// TranscriptMerger manages the merging of speech segments based on timing.
type TranscriptMerger struct {
	// Merge threshold: segments within this duration are merged
	mergeThreshold time.Duration

	// Context management
	maxContext  int      // Maximum number of recent texts to keep
	recentTexts []string // Rolling buffer of recent texts

	// Last segment state
	lastID   string
	lastText string
	lastEnd  time.Time

	// ID generation
	idCounter atomic.Uint64
	count     int
}

// NewTranscriptMerger creates a new transcript merger.
func NewTranscriptMerger(mergeThreshold time.Duration, maxContext int) *TranscriptMerger {
	return &TranscriptMerger{
		mergeThreshold: mergeThreshold,
		maxContext:     maxContext,
		recentTexts:    make([]string, 0, maxContext),
	}
}

// MergeResult represents the result of processing a new transcript segment.
type MergeResult struct {
	ID           string   // Transcript ID (new or merged)
	FullText     string   // Complete text (original or merged)
	IsMerged     bool     // Whether this was merged with previous segment
	Context      []string // Recent context texts (excluding current)
	IsNewSegment bool     // Whether a new segment was created
}

// Process processes a new transcript segment and determines if it should be merged.
func (m *TranscriptMerger) Process(newText string, timestamp time.Time) MergeResult {
	result := MergeResult{
		Context: make([]string, 0),
	}

	// Determine if we should merge with the previous segment
	shouldMerge := false
	if m.lastID != "" {
		gap := timestamp.Sub(m.lastEnd)
		if gap < m.mergeThreshold {
			shouldMerge = true
		}
	}

	if shouldMerge {
		// Merge with previous segment
		result.ID = m.lastID
		result.FullText = m.lastText + " " + newText
		result.IsMerged = true
		result.IsNewSegment = false

		// Update recent context: replace last entry with merged text
		if len(m.recentTexts) > 0 {
			m.recentTexts[len(m.recentTexts)-1] = result.FullText
		}
	} else {
		// Create new segment
		id := m.idCounter.Add(1)
		result.ID = fmt.Sprintf("lt-%d", id)
		result.FullText = newText
		result.IsMerged = false
		result.IsNewSegment = true

		// Add to recent context
		m.recentTexts = append(m.recentTexts, result.FullText)
		if len(m.recentTexts) > m.maxContext {
			m.recentTexts = m.recentTexts[1:]
		}
		m.count++
	}

	// Build context (excluding current segment)
	for _, t := range m.recentTexts {
		if t != result.FullText {
			result.Context = append(result.Context, t)
		}
	}

	// Update state
	m.lastID = result.ID
	m.lastText = result.FullText
	m.lastEnd = timestamp

	return result
}

// GetRecentContext returns the recent context texts.
func (m *TranscriptMerger) GetRecentContext() []string {
	contextCopy := make([]string, len(m.recentTexts))
	copy(contextCopy, m.recentTexts)
	return contextCopy
}

// Reset clears all state.
func (m *TranscriptMerger) Reset() {
	m.lastID = ""
	m.lastText = ""
	m.lastEnd = time.Time{}
	m.recentTexts = m.recentTexts[:0]
	m.count = 0
}

// Count returns the total number of segments created.
func (m *TranscriptMerger) Count() int {
	return m.count
}
