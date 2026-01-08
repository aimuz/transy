// Package app provides the core application service for Wails bindings.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"go.aimuz.me/transy/cache"
	"go.aimuz.me/transy/clipboard"
	"go.aimuz.me/transy/config"
	"go.aimuz.me/transy/hotkey"
	"go.aimuz.me/transy/internal/types"
	"go.aimuz.me/transy/langdetect"
	"go.aimuz.me/transy/livetranslate"
	"go.aimuz.me/transy/llm"
	"go.aimuz.me/transy/ocr"
	"go.aimuz.me/transy/screenshot"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Service provides application functionality bound to Wails.
// This struct focuses on orchestration; business logic lives in sub-components.
type Service struct {
	cfg    *config.Config
	cache  *cache.Cache
	hotkey *hotkey.HotkeyManager

	// UI references - set via Init
	app    *application.App
	window application.Window

	// Components with proper synchronization
	translator *Translator
	live       LiveAdapter
	audio      AudioAdapter

	// Version info (set by caller)
	version string
}

// New creates a new Service. Call Init() after Wails app is created.
func New(version string) *Service {
	return &Service{version: version}
}

// GetVersion returns the application version.
func (s *Service) GetVersion() string {
	return s.version
}

// Init initializes the service with app and window references.
// Must be called after Wails application is created.
func (s *Service) Init(app *application.App, window application.Window) {
	s.app = app
	s.window = window

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		cfg = &config.Config{}
	}
	s.cfg = cfg

	// Initialize cache
	s.setupCache()

	// Initialize translator
	s.translator = NewTranslator(s.cache)

	// Setup hotkey
	s.setupHotkey()
}

// Shutdown cleans up resources.
func (s *Service) Shutdown() {
	if s.hotkey != nil {
		s.hotkey.Stop()
	}
	_ = s.live.Stop()
	_ = s.audio.Stop()
	if s.cache != nil {
		if err := s.cache.Close(); err != nil {
			slog.Error("close cache", "error", err)
		}
	}
}

func (s *Service) setupCache() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		slog.Error("get config dir for cache", "error", err)
		return
	}

	cachePath := filepath.Join(configDir, "transy", "cache")
	c, err := cache.New(cachePath)
	if err != nil {
		slog.Error("init cache", "error", err)
		return
	}
	s.cache = c
	slog.Info("cache initialized", "path", cachePath)
}

func (s *Service) setupHotkey() {
	s.hotkey = hotkey.NewHotkeyManager(
		func() { s.ToggleWindowVisibility() },
		func() {
			go func() {
				if _, err := s.TakeScreenshotAndOCR(); err != nil {
					slog.Error("ocr screenshot", "error", err)
				}
			}()
		},
	)

	s.hotkey.SetStatusCallback(func(granted bool) {
		s.emit(EventAccessibilityPerm, granted)
		if granted {
			slog.Info("accessibility permission granted")
		} else {
			slog.Warn("accessibility permission denied")
		}
	})

	if err := s.hotkey.Start(); err != nil {
		slog.Error("start hotkey", "error", err)
	}
}

// emit is a safe wrapper around app.Event.Emit
func (s *Service) emit(name string, data any) {
	if s.app != nil {
		s.app.Event.Emit(name, data)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Live Translation
// ─────────────────────────────────────────────────────────────────────────────

// StartLiveTranslation starts real-time audio translation.
func (s *Service) StartLiveTranslation(sourceLang, targetLang string) error {
	cfg := s.buildLiveConfig()

	translator, err := livetranslate.New(cfg)
	if err != nil {
		return err
	}

	if err := s.live.Start(context.Background(), translator, sourceLang, targetLang); err != nil {
		return err
	}

	// Forward events in background
	go s.live.ForwardEvents(s.emit, s.translateAndEmit)

	return nil
}

func (s *Service) buildLiveConfig() livetranslate.Config {
	speechCfg := s.cfg.GetSpeechConfig()

	cfg := livetranslate.Config{}
	if speechCfg != nil && speechCfg.CredentialID != "" {
		if cred := s.cfg.GetCredential(speechCfg.CredentialID); cred != nil {
			cfg.APIKey = cred.APIKey
		}
		cfg.Model = speechCfg.Model
		cfg.SystemPrompt = "You are a professional translator. Translate the input audio text into the target language directly. Output only the translated text."
		cfg.Temperature = 0.6
	}
	return cfg
}

func (s *Service) translateAndEmit(t types.LiveTranscript) {
	result, err := s.TranslateWithLLM(types.TranslateRequest{
		Text:       t.SourceText,
		SourceLang: t.SourceLang,
		TargetLang: t.TargetLang,
	})
	if err != nil {
		slog.Warn("async translate failed", "id", t.ID, "error", err)
		return
	}

	t.TargetText = result.Text
	s.emit(EventLiveTranscript, t)
}

// StopLiveTranslation stops real-time audio translation.
func (s *Service) StopLiveTranslation() error {
	return s.live.Stop()
}

// GetLiveStatus returns the current live translation status.
func (s *Service) GetLiveStatus() types.LiveStatus {
	return s.live.Status()
}

// ─────────────────────────────────────────────────────────────────────────────
// Audio Capture
// ─────────────────────────────────────────────────────────────────────────────

// StartAudioCapture starts capturing system audio.
func (s *Service) StartAudioCapture() error {
	return s.audio.Start(s.emit)
}

// StopAudioCapture stops the audio capture.
func (s *Service) StopAudioCapture() error {
	return s.audio.Stop()
}

// ─────────────────────────────────────────────────────────────────────────────
// Window & Clipboard
// ─────────────────────────────────────────────────────────────────────────────

// ToggleWindowVisibility shows the window with clipboard text.
func (s *Service) ToggleWindowVisibility() {
	text, err := clipboard.GetText(s.app)
	if err != nil {
		slog.Error("get clipboard", "error", err)
		return
	}
	s.showWindow()
	if text != "" {
		s.emit(EventSetClipboard, text)
	}
}

func (s *Service) showWindow() {
	if s.window != nil {
		s.window.Show()
		s.window.Focus()
	}
}

// TakeScreenshotAndOCR captures a screenshot and performs OCR.
func (s *Service) TakeScreenshotAndOCR() (string, error) {
	if s.window != nil {
		s.window.Hide()
	}
	time.Sleep(100 * time.Millisecond)

	if !screenshot.HasPermission() {
		screenshot.RequestPermission()
		return "", fmt.Errorf("screen recording permission required")
	}

	imagePath, err := screenshot.CaptureInteractive()
	if err != nil {
		if s.window != nil {
			s.window.Show()
		}
		return "", fmt.Errorf("capture screenshot: %w", err)
	}
	defer os.Remove(imagePath)

	text, err := ocr.RecognizeText(imagePath)
	if err != nil {
		if s.window != nil {
			s.window.Show()
		}
		return "", fmt.Errorf("recognize text: %w", err)
	}

	s.showWindow()
	if text != "" {
		s.emit(EventSetClipboard, text)
	}
	return text, nil
}

// GetAccessibilityPermission returns whether accessibility is enabled.
func (s *Service) GetAccessibilityPermission() bool {
	return hotkey.IsAccessibilityEnabled(false)
}

// GetScreenRecordingPermission returns whether screen recording is permitted.
func (s *Service) GetScreenRecordingPermission() bool {
	return screenshot.HasPermission()
}

// RequestScreenRecordingPermission requests screen recording permission.
func (s *Service) RequestScreenRecordingPermission() {
	screenshot.RequestPermission()
}

// ─────────────────────────────────────────────────────────────────────────────
// Translation
// ─────────────────────────────────────────────────────────────────────────────

// TranslateWithLLM translates text using the active provider.
func (s *Service) TranslateWithLLM(req types.TranslateRequest) (types.TranslateResult, error) {
	profile := s.cfg.GetActiveTranslationProfile()
	if profile == nil {
		return types.TranslateResult{}, fmt.Errorf("no active translation profile")
	}

	cred := s.cfg.GetCredential(profile.CredentialID)
	if cred == nil {
		return types.TranslateResult{}, fmt.Errorf("credential not found: %s", profile.CredentialID)
	}

	completer := llm.NewCompleter(cred.Type, cred.APIKey, cred.BaseURL, profile.Model, llm.Options{
		MaxTokens:       profile.MaxTokens,
		Temperature:     profile.Temperature,
		DisableThinking: profile.DisableThinking,
	})

	return s.translator.Translate(context.Background(), completer, TranslateProfile{
		Name:         profile.Name,
		Model:        profile.Model,
		SystemPrompt: profile.SystemPrompt,
	}, req)
}

// ─────────────────────────────────────────────────────────────────────────────
// API Credential Management
// ─────────────────────────────────────────────────────────────────────────────

// GetCredentials returns all API credentials.
func (s *Service) GetCredentials() []types.APICredential {
	return s.cfg.GetCredentials()
}

// AddCredential adds a new API credential.
func (s *Service) AddCredential(cred types.APICredential) error {
	return s.cfg.AddCredential(cred)
}

// UpdateCredential updates an existing credential.
func (s *Service) UpdateCredential(id string, cred types.APICredential) error {
	return s.cfg.UpdateCredential(id, cred)
}

// RemoveCredential removes a credential by ID.
func (s *Service) RemoveCredential(id string) error {
	return s.cfg.RemoveCredential(id)
}

// ─────────────────────────────────────────────────────────────────────────────
// Translation Profile Management
// ─────────────────────────────────────────────────────────────────────────────

// GetTranslationProfiles returns all translation profiles.
func (s *Service) GetTranslationProfiles() []types.TranslationProfile {
	return s.cfg.GetTranslationProfiles()
}

// GetActiveTranslationProfile returns the currently active translation profile.
func (s *Service) GetActiveTranslationProfile() *types.TranslationProfile {
	return s.cfg.GetActiveTranslationProfile()
}

// AddTranslationProfile adds a new translation profile.
func (s *Service) AddTranslationProfile(profile types.TranslationProfile) error {
	return s.cfg.AddTranslationProfile(profile)
}

// UpdateTranslationProfile updates an existing translation profile.
func (s *Service) UpdateTranslationProfile(id string, profile types.TranslationProfile) error {
	return s.cfg.UpdateTranslationProfile(id, profile)
}

// RemoveTranslationProfile removes a translation profile by ID.
func (s *Service) RemoveTranslationProfile(id string) error {
	return s.cfg.RemoveTranslationProfile(id)
}

// SetTranslationProfileActive sets a translation profile as active.
func (s *Service) SetTranslationProfileActive(id string) error {
	return s.cfg.SetTranslationProfileActive(id)
}

// ─────────────────────────────────────────────────────────────────────────────
// Speech Configuration
// ─────────────────────────────────────────────────────────────────────────────

// GetSpeechConfig returns the speech service configuration.
func (s *Service) GetSpeechConfig() *types.SpeechConfig {
	return s.cfg.GetSpeechConfig()
}

// SetSpeechConfig sets the speech service configuration.
func (s *Service) SetSpeechConfig(cfg types.SpeechConfig) error {
	return s.cfg.SetSpeechConfig(cfg)
}

// ─────────────────────────────────────────────────────────────────────────────
// Language Settings
// ─────────────────────────────────────────────────────────────────────────────

// GetDefaultLanguages returns the default language mappings.
func (s *Service) GetDefaultLanguages() map[string]string {
	return s.cfg.DefaultLanguages
}

// SetDefaultLanguage sets the default target language for a source.
func (s *Service) SetDefaultLanguage(src, dst string) error {
	if s.cfg.DefaultLanguages == nil {
		s.cfg.DefaultLanguages = make(map[string]string)
	}
	s.cfg.DefaultLanguages[src] = dst
	return s.cfg.Save()
}

// DetectLanguage detects the language of the given text.
func (s *Service) DetectLanguage(text string) types.DetectResult {
	code, name := langdetect.Detect(text)

	target := "en"
	if code != "auto" && s.cfg.DefaultLanguages != nil {
		if t, ok := s.cfg.DefaultLanguages[code]; ok {
			target = t
		}
	}

	return types.DetectResult{
		Code:          code,
		Name:          name,
		DefaultTarget: target,
	}
}
