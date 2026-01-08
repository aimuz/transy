// Package app provides the core application service for Wails bindings.
package app

// Event names for frontend communication.
const (
	EventLiveTranscript    = "live-transcript"
	EventVADUpdate         = "live-vad-update"
	EventSetClipboard      = "set-clipboard-text"
	EventAccessibilityPerm = "accessibility-permission"
	EventTranslateChunk    = "translate-chunk"
)
