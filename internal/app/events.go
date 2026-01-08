// Package app provides the core application service for Wails bindings.
package app

// Event names for frontend communication.
const (
	EventLiveTranscript    = "live-transcript"
	EventVADUpdate         = "live-vad-update"
	EventAudioSamples      = "audio-samples"
	EventSetClipboard      = "set-clipboard-text"
	EventAccessibilityPerm = "accessibility-permission"
)

// AudioSamples is a typed event for audio data emission.
// Fields ordered by size for optimal memory layout.
type AudioSamples struct {
	Samples   []float32 `json:"samples"`   // 24 bytes (slice header)
	Timestamp int64     `json:"timestamp"` // 8 bytes
	Seq       int       `json:"seq"`       // 8 bytes on 64-bit
}
