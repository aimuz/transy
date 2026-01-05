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
	mu    sync.RWMutex
	ready bool
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
func (s *SpeechDarwin) RequiresSetup() bool { return false }
func (s *SpeechDarwin) SetupProgress() int  { return 100 }

func (s *SpeechDarwin) IsReady() bool {
	// Always return true - actual authorization check happens in Transcribe.
	// This avoids calling into Objective-C from the main thread during init.
	return true
}

func (s *SpeechDarwin) Setup(_ func(percent int)) error {
	// Authorization will be requested automatically when using speech recognition.
	// We don't block here to avoid main thread deadlock.
	return nil
}

// Transcribe converts audio samples to text using macOS Speech Framework.
// audio: PCM float32 samples at 16000 Hz
// language: source language code (e.g., "en", "zh", "ja")
func (s *SpeechDarwin) Transcribe(audio []float32, language string) (*TranscribeResult, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("speech recognition not authorized")
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
