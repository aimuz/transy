package livetranslate

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.aimuz.me/transy/internal/types"
)

// TranscriptManager manages transcript segments, merging, context, and timeline.
// It follows the principle of single responsibility and provides a clean interface.
type TranscriptManager struct {
	mu sync.Mutex

	// Session info
	sessionStart time.Time
	sourceLang   string
	targetLang   string

	// ID generation
	idCounter atomic.Uint64
	count     int

	// Merge settings
	mergeThreshold time.Duration // Merge segments if gap < threshold

	// Context management for translation
	maxContext  int      // Maximum number of recent texts for context
	recentTexts []string // Recent source texts for translation context

	// Last segment for merging
	lastSegment *Segment
}

// Segment represents a transcript segment with timing.
type Segment struct {
	ID         string
	SourceText string
	TargetText string // May be empty initially, filled when translation completes
	StartTime  int64  // ms since session start
	EndTime    int64  // ms since session start
	Confidence float64
	IsFinal    bool
}

// NewTranscriptManager creates a new transcript manager.
func NewTranscriptManager(sessionStart time.Time, sourceLang, targetLang string, mergeThreshold time.Duration, maxContext int) *TranscriptManager {
	return &TranscriptManager{
		sessionStart:   sessionStart,
		sourceLang:     sourceLang,
		targetLang:     targetLang,
		mergeThreshold: mergeThreshold,
		maxContext:     maxContext,
		recentTexts:    make([]string, 0, maxContext),
	}
}

// TranscriptProcessResult contains the result of processing a new transcript.
type TranscriptProcessResult struct {
	Segment      *Segment
	Context      string // Context for translation (recent texts joined)
	IsNewSegment bool   // true if new segment created, false if merged
}

// Process processes a new transcript text with timing information.
// It determines whether to merge with the previous segment or create a new one.
func (tm *TranscriptManager) Process(text string, startTime, duration int64, confidence float64) TranscriptProcessResult {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	endTime := startTime + duration

	// Determine if we should merge with the previous segment
	shouldMerge := false
	if tm.lastSegment != nil && !tm.lastSegment.IsFinal {
		gap := startTime - tm.lastSegment.EndTime
		if gap < tm.mergeThreshold.Milliseconds() {
			shouldMerge = true
		}
	}

	var segment *Segment

	if shouldMerge {
		// Merge with previous segment
		segment = tm.lastSegment
		segment.SourceText = segment.SourceText + " " + text
		segment.EndTime = endTime
		segment.Confidence = (segment.Confidence + confidence) / 2 // Average confidence

		// Update context: replace last entry
		if len(tm.recentTexts) > 0 {
			tm.recentTexts[len(tm.recentTexts)-1] = segment.SourceText
		}
	} else {
		// Create new segment
		id := tm.idCounter.Add(1)
		segment = &Segment{
			ID:         fmt.Sprintf("lt-%d", id),
			SourceText: text,
			TargetText: "", // Will be filled by translation
			StartTime:  startTime,
			EndTime:    endTime,
			Confidence: confidence,
			IsFinal:    false,
		}

		tm.lastSegment = segment
		tm.count++

		// Add to context
		tm.recentTexts = append(tm.recentTexts, text)
		if len(tm.recentTexts) > tm.maxContext {
			tm.recentTexts = tm.recentTexts[1:]
		}
	}

	// Build context (exclude current segment)
	contextTexts := make([]string, 0, len(tm.recentTexts))
	for _, t := range tm.recentTexts {
		if t != segment.SourceText {
			contextTexts = append(contextTexts, t)
		}
	}
	context := strings.Join(contextTexts, " ")

	return TranscriptProcessResult{
		Segment:      segment,
		Context:      context,
		IsNewSegment: !shouldMerge,
	}
}

// UpdateTranslation updates the translation for a segment.
func (tm *TranscriptManager) UpdateTranslation(segmentID, targetText string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.lastSegment != nil && tm.lastSegment.ID == segmentID {
		tm.lastSegment.TargetText = targetText
	}
}

// MarkFinal marks the current segment as final.
func (tm *TranscriptManager) MarkFinal() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.lastSegment != nil {
		tm.lastSegment.IsFinal = true
	}
}

// ToLiveTranscript converts a Segment to types.LiveTranscript.
func (tm *TranscriptManager) ToLiveTranscript(seg *Segment) types.LiveTranscript {
	return types.LiveTranscript{
		ID:         seg.ID,
		SourceText: seg.SourceText,
		TargetText: seg.TargetText,
		SourceLang: tm.sourceLang,
		TargetLang: tm.targetLang,
		StartTime:  seg.StartTime,
		EndTime:    seg.EndTime,
		Timestamp:  time.Now().UnixMilli(),
		IsFinal:    seg.IsFinal,
		Confidence: seg.Confidence,
		// Backward compatibility
		Text:       seg.SourceText,
		Translated: seg.TargetText,
	}
}

// Count returns the total number of segments created.
func (tm *TranscriptManager) Count() int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.count
}

// Reset clears all state for a new session.
func (tm *TranscriptManager) Reset(sessionStart time.Time, sourceLang, targetLang string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.sessionStart = sessionStart
	tm.sourceLang = sourceLang
	tm.targetLang = targetLang
	tm.count = 0
	tm.lastSegment = nil
	tm.recentTexts = tm.recentTexts[:0]
}
