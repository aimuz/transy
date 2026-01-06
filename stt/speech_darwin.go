//go:build darwin

package stt

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=10.15
#cgo LDFLAGS: -framework Speech -framework Foundation -framework AVFoundation

#include <stdlib.h>

// Declaration of the Objective-C functions implemented in speech_darwin.m
extern char* recognizeSpeech(float* samples, int sampleCount, int sampleRate, const char* language);
extern int isSpeechRecognitionAvailable(const char* language);
extern int requestSpeechRecognitionAuthorization(void);
extern int checkSpeechRecognitionAuthorizationStatus(void);
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// SpeechDarwin implements the Provider interface using macOS Speech Framework.
// This provides low-latency on-device speech recognition.
type SpeechDarwin struct {
	mu            sync.RWMutex
	statusChecked bool
	authorized    bool
	appStarted    bool // Set to true after app initialization is complete
}

// NewSpeechDarwin creates a new macOS Speech provider.
// Note: Authorization is requested lazily to avoid blocking the main thread.
func NewSpeechDarwin() (*SpeechDarwin, error) {
	s := &SpeechDarwin{}
	// Don't request authorization here - it would deadlock on main thread.
	// Authorization will be checked/requested lazily when needed.
	return s, nil
}

func (s *SpeechDarwin) Name() string        { return "speech-darwin" }
func (s *SpeechDarwin) DisplayName() string { return "macOS 语音识别 (低延迟)" }
func (s *SpeechDarwin) IsLocal() bool       { return true }
func (s *SpeechDarwin) RequiresSetup() bool {
	// Speech recognition may require authorization but we can't check safely
	// during startup. Return false and let Transcribe() handle authorization.
	return true
}
func (s *SpeechDarwin) SetupProgress() int { return 100 }

func (s *SpeechDarwin) IsReady() bool {
	s.mu.RLock()
	appStarted := s.appStarted
	statusChecked := s.statusChecked
	authorized := s.authorized
	s.mu.RUnlock()

	// Return cached result if already checked
	if statusChecked {
		return authorized
	}

	// Don't check during initial app startup - defer to later.
	// This avoids cgo calls that can crash during AppKit initialization.
	if !appStarted {
		return false
	}

	// Do a quick non-blocking check
	status := int(C.checkSpeechRecognitionAuthorizationStatus())
	if status == 3 { // Authorized
		s.mu.Lock()
		s.statusChecked = true
		s.authorized = true
		s.mu.Unlock()
		return true
	}
	if status == 1 || status == 2 { // Denied or Restricted
		s.mu.Lock()
		s.statusChecked = true
		s.authorized = false
		s.mu.Unlock()
		return false
	}
	// Status is NotDetermined - don't trigger auth here to avoid crashes
	// User should call Setup() or the provider will be marked as requiring setup
	return false
}

// MarkAppStarted should be called after the app has fully initialized.
// This enables IsReady() to perform actual status checks.
func (s *SpeechDarwin) MarkAppStarted() {
	s.mu.Lock()
	s.appStarted = true
	s.mu.Unlock()
}

// checkAndRequestAuth checks authorization status and triggers request if needed.
// Returns true if authorized, false otherwise.
func (s *SpeechDarwin) checkAndRequestAuth() bool {
	s.mu.Lock()
	if s.statusChecked {
		auth := s.authorized
		s.mu.Unlock()
		return auth
	}
	s.mu.Unlock()

	// 0: NotDetermined, 1: Denied, 2: Restricted, 3: Authorized
	status := int(C.checkSpeechRecognitionAuthorizationStatus())
	if status == 3 {
		s.mu.Lock()
		s.statusChecked = true
		s.authorized = true
		s.mu.Unlock()
		return true
	}

	if status == 0 { // NotDetermined
		// Trigger authorization request - this is async and may show a dialog
		C.requestSpeechRecognitionAuthorization()
		// Don't cache status yet - user hasn't made a choice
		return false
	}

	// Denied or Restricted
	s.mu.Lock()
	s.statusChecked = true
	s.authorized = false
	s.mu.Unlock()
	return false
}

func (s *SpeechDarwin) Setup(progress func(percent int)) error {
	// Simply check if authorized - don't try to trigger system dialogs
	// This avoids crashes from cgo/AppKit conflicts
	if progress != nil {
		progress(50)
	}

	s.checkAndRequestAuth()

	if progress != nil {
		progress(100)
	}
	return nil
}

// Transcribe converts audio samples to text using macOS Speech Framework.
// audio: PCM float32 samples at 16000 Hz
// language: source language code (e.g., "en", "zh", "ja")
func (s *SpeechDarwin) Transcribe(audio []float32, language string) (*TranscribeResult, error) {
	// Check and request authorization at actual use time (not during startup)
	if !s.checkAndRequestAuth() {
		return nil, fmt.Errorf("speech recognition permission denied or not authorized")
	}

	if len(audio) == 0 {
		return &TranscribeResult{Text: ""}, nil
	}

	// Convert language code to locale identifier
	locale := languageToLocale(language)

	// Check if language is available
	cLocale := C.CString(locale)
	defer C.free(unsafe.Pointer(cLocale))

	if C.isSpeechRecognitionAvailable(cLocale) == 0 {
		return nil, fmt.Errorf("speech recognition not available for language: %s", language)
	}

	// Call native recognition
	cResult := C.recognizeSpeech(
		(*C.float)(unsafe.Pointer(&audio[0])),
		C.int(len(audio)),
		C.int(16000),
		cLocale,
	)

	if cResult == nil {
		return &TranscribeResult{Text: ""}, nil
	}
	defer C.free(unsafe.Pointer(cResult))

	text := C.GoString(cResult)

	return &TranscribeResult{
		Text:       strings.TrimSpace(text),
		Language:   language,
		Confidence: 0.9,
	}, nil
}

func (s *SpeechDarwin) Close() error {
	return nil
}

// languageToLocale converts language codes to macOS locale identifiers.
func languageToLocale(lang string) string {
	// Handle auto-detect
	if lang == "" || lang == "auto" {
		return "en-US" // Default to English for auto
	}

	// Common language code mappings
	locales := map[string]string{
		"en": "en-US",
		"zh": "zh-CN",
		"ja": "ja-JP",
		"ko": "ko-KR",
		"fr": "fr-FR",
		"de": "de-DE",
		"es": "es-ES",
		"it": "it-IT",
		"pt": "pt-BR",
		"ru": "ru-RU",
		"ar": "ar-SA",
	}

	if locale, ok := locales[lang]; ok {
		return locale
	}

	// If already a full locale, use as is
	if strings.Contains(lang, "-") || strings.Contains(lang, "_") {
		return lang
	}

	// Default: append common region
	return lang + "-" + strings.ToUpper(lang)
}
