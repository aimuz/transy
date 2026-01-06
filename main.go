package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
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
	"go.aimuz.me/transy/stt"
)

//go:embed all:frontend/dist
var assets embed.FS

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// App is the main application service bound to Wails.
type App struct {
	app    *application.App
	window application.Window
	cfg    *config.Config
	hotkey *hotkey.HotkeyManager
	cache  *cache.Cache

	// Live translation
	liveService types.LiveTranslator
	sttRegistry *stt.Registry
}

func NewApp() *App {
	return &App{}
}

// ─────────────────────────────────────────────────────────────────────────────
// Initialization (called from main)
// ─────────────────────────────────────────────────────────────────────────────

// Init initializes the service with references to app and window.
func (a *App) Init(app *application.App, window application.Window) {
	a.app = app
	a.window = window

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		cfg = &config.Config{}
	}
	a.cfg = cfg

	// Initialize cache
	a.setupCache()

	// Initialize STT providers
	a.setupSTT()

	a.setupHotkey()
}

// Shutdown cleans up resources.
func (a *App) Shutdown() {
	if a.hotkey != nil {
		a.hotkey.Stop()
	}
	if a.liveService != nil {
		a.liveService.Stop()
	}
	if a.sttRegistry != nil {
		a.sttRegistry.Close()
	}
	if a.cache != nil {
		if err := a.cache.Close(); err != nil {
			slog.Error("close cache", "error", err)
		}
	}
}

func (a *App) setupCache() {
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
	a.cache = c
	slog.Info("cache initialized", "path", cachePath)
}

func (a *App) setupSTT() {
	a.sttRegistry = stt.NewRegistry()

	// Register macOS Speech provider first (lowest latency, on-device)
	// Note: We don't call any cgo/Speech Framework functions here to avoid
	// crashes during AppKit initialization. Authorization will be checked
	// lazily when the provider is actually used.
	speechProvider, err := stt.NewSpeechDarwin()
	if err != nil {
		slog.Error("init macOS Speech provider", "error", err)
	} else {
		speechProvider.MarkAppStarted() // Allow cgo calls, but don't make any yet
		a.sttRegistry.Register(speechProvider)
		slog.Info("registered macOS Speech provider (authorization checked on first use)")
	}

	// Register API provider if configured via SpeechConfig
	speechCfg := a.cfg.GetSpeechConfig()
	if speechCfg != nil && speechCfg.Enabled && speechCfg.CredentialID != "" {
		cred := a.cfg.GetCredential(speechCfg.CredentialID)
		if cred != nil {
			model := speechCfg.Model
			if model == "" {
				model = "whisper-1"
			}
			apiProvider := stt.NewWhisperAPI(stt.WhisperAPIConfig{
				APIKey:  cred.APIKey,
				BaseURL: cred.BaseURL,
				Model:   model,
			})
			a.sttRegistry.Register(apiProvider)
			slog.Info("registered OpenAI Whisper API provider", "credential", cred.Name)
		}
	}

	// Register local whisper provider (may not be ready if whisper.cpp not installed)
	localProvider, err := stt.NewWhisperLocal(stt.WhisperLocalConfig{
		ModelSize: "base",
	})
	if err != nil {
		slog.Error("init whisper local", "error", err)
	} else {
		a.sttRegistry.Register(localProvider)
		if localProvider.HasBinary() {
			slog.Info("registered Whisper Local provider", "ready", localProvider.IsReady())
		} else {
			slog.Warn("Whisper Local registered but whisper.cpp binary not found - install with: brew install whisper-cpp")
		}
	}

	slog.Info("STT providers initialized", "count", len(a.sttRegistry.List()))
}

// ─────────────────────────────────────────────────────────────────────────────
// Live Translation
// ─────────────────────────────────────────────────────────────────────────────

// StartLiveTranslation starts real-time audio translation.
// Uses factory pattern - main.go doesn't need to know implementation details.
func (a *App) StartLiveTranslation(sourceLang, targetLang string) error {
	cfg := a.buildTranslatorConfig()

	translator, err := livetranslate.NewOrReuse(a.liveService, cfg)
	if err != nil {
		return err
	}
	a.liveService = translator

	if err := a.liveService.Start(context.Background(), sourceLang, targetLang); err != nil {
		return err
	}

	// Start goroutines to forward transcripts and errors to frontend
	go a.forwardTranscripts()
	go a.forwardErrors()

	return nil
}

// forwardTranscripts forwards transcript events from the service to the frontend.
func (a *App) forwardTranscripts() {
	if a.liveService == nil {
		return
	}
	for transcript := range a.liveService.Transcripts() {
		if a.app != nil {
			a.app.Event.Emit("live-transcript", transcript)
		}
	}
}

// forwardErrors forwards errors from the service to the log.
func (a *App) forwardErrors() {
	if a.liveService == nil {
		return
	}
	for err := range a.liveService.Errors() {
		slog.Error("live translation error", "error", err)
	}
}

// buildTranslatorConfig builds factory configuration from app settings.
func (a *App) buildTranslatorConfig() livetranslate.FactoryConfig {
	speechCfg := a.cfg.GetSpeechConfig()

	cfg := livetranslate.FactoryConfig{}

	if speechCfg == nil {
		cfg.Mode = "transcription"
		cfg.STTProvider = a.findSTTProvider()
		cfg.TranslateFunc = a.translateFunc
		return cfg
	}

	cfg.Mode = speechCfg.Mode

	if cfg.Mode == "realtime" {
		if cred := a.cfg.GetCredential(speechCfg.CredentialID); cred != nil {
			cfg.APIKey = cred.APIKey
		}
		cfg.Model = speechCfg.Model
		cfg.SystemPrompt = "You are a professional translator. Translate the input audio text into the target language directly. Output only the translated text."
		cfg.Temperature = 0.6
	} else {
		cfg.STTProvider = a.findSTTProvider()
		cfg.TranslateFunc = a.translateFunc
	}

	return cfg
}

// translateFunc translates text using the configured LLM.
func (a *App) translateFunc(text, ctx, srcLang, dstLang string) (string, error) {
	result, err := a.TranslateWithLLM(types.TranslateRequest{
		Text:       text,
		SourceLang: srcLang,
		TargetLang: dstLang,
		Context:    ctx,
	})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// findSTTProvider finds an available STT provider.
func (a *App) findSTTProvider() stt.Provider {
	if a.cfg.STTProvider != "" {
		if p := a.sttRegistry.Get(a.cfg.STTProvider); p != nil {
			return p
		}
	}
	if p := a.sttRegistry.Get("whisper-local"); p != nil {
		return p
	}
	providers := a.sttRegistry.List()
	if len(providers) > 0 {
		return providers[0]
	}
	return nil
}

// StopLiveTranslation stops real-time audio translation.
func (a *App) StopLiveTranslation() error {
	if a.liveService == nil {
		return nil
	}
	return a.liveService.Stop()
}

// GetLiveStatus returns the current live translation status.
func (a *App) GetLiveStatus() types.LiveStatus {
	if a.liveService == nil {
		return types.LiveStatus{}
	}
	return a.liveService.Status()
}

// GetSTTProviders returns available STT providers.
func (a *App) GetSTTProviders() []types.STTProviderInfo {
	if a.sttRegistry == nil {
		return nil
	}

	providers := a.sttRegistry.List()
	result := make([]types.STTProviderInfo, len(providers))
	for i, p := range providers {
		result[i] = types.STTProviderInfo{
			Name:          p.Name(),
			DisplayName:   p.DisplayName(),
			IsLocal:       p.IsLocal(),
			RequiresSetup: p.RequiresSetup(),
			SetupProgress: p.SetupProgress(),
			IsReady:       p.IsReady(),
		}
	}
	return result
}

// SetSTTProvider sets the active STT provider for live translation.
func (a *App) SetSTTProvider(name string) error {
	if a.sttRegistry == nil {
		return fmt.Errorf("STT registry not initialized")
	}

	provider := a.sttRegistry.Get(name)
	if provider == nil {
		return fmt.Errorf("provider not found: %s", name)
	}

	// Save preference
	a.cfg.STTProvider = name
	if err := a.cfg.Save(); err != nil {
		slog.Error("save config", "error", err)
	}

	if a.liveService != nil {
		if srv, ok := a.liveService.(*livetranslate.Service); ok {
			srv.SetSTTProvider(provider)
		}
	}
	return nil
}

// SetupSTTProvider downloads/initializes an STT provider.
func (a *App) SetupSTTProvider(name string) error {
	if a.sttRegistry == nil {
		return fmt.Errorf("STT registry not initialized")
	}

	provider := a.sttRegistry.Get(name)
	if provider == nil {
		return fmt.Errorf("provider not found: %s", name)
	}

	// Run setup in background, emit progress events
	go func() {
		err := provider.Setup(func(percent int) {
			if a.app != nil {
				a.app.Event.Emit("stt-setup-progress", map[string]interface{}{
					"provider": name,
					"progress": percent,
				})
			}
		})
		if err != nil {
			slog.Error("STT setup failed", "provider", name, "error", err)
			if a.app != nil {
				a.app.Event.Emit("stt-setup-error", map[string]interface{}{
					"provider": name,
					"error":    err.Error(),
				})
			}
		} else {
			if a.app != nil {
				a.app.Event.Emit("stt-setup-complete", name)
			}
		}
	}()

	return nil
}

// GetSTTSetupProgress returns the setup progress for a provider.
func (a *App) GetSTTSetupProgress(name string) int {
	if a.sttRegistry == nil {
		return -1
	}

	provider := a.sttRegistry.Get(name)
	if provider == nil {
		return -1
	}

	return provider.SetupProgress()
}

func (a *App) setupHotkey() {
	a.hotkey = hotkey.NewHotkeyManager(
		func() {
			a.ToggleWindowVisibility()
		},
		func() {
			// Run in goroutine to not block the hotkey listener
			go func() {
				if _, err := a.TakeScreenshotAndOCR(); err != nil {
					slog.Error("ocr screenshot", "error", err)
				}
			}()
		},
	)

	a.hotkey.SetStatusCallback(func(granted bool) {
		if a.app != nil {
			a.app.Event.Emit("accessibility-permission", granted)
		}
		if granted {
			slog.Info("accessibility permission granted")
		} else {
			slog.Warn("accessibility permission denied")
		}
	})

	if err := a.hotkey.Start(); err != nil {
		slog.Error("start hotkey", "error", err)
	}
}

// TakeScreenshotAndOCR captures a screenshot and performs OCR.
// Returns the recognized text.
func (a *App) TakeScreenshotAndOCR() (string, error) {
	// Hide window to allow capturing screen behind it
	if a.window != nil {
		a.window.Hide()
	}

	// Give a little time for window to hide
	time.Sleep(100 * time.Millisecond)

	// Check screen recording permission
	if !screenshot.HasPermission() {
		screenshot.RequestPermission()
		return "", fmt.Errorf("screen recording permission required")
	}

	imagePath, err := screenshot.CaptureInteractive()
	if err != nil {
		// If cancelled or failed, show window again if not active
		if a.window != nil {
			a.window.Show()
		}
		return "", fmt.Errorf("capture screenshot: %w", err)
	}
	defer os.Remove(imagePath) // Clean up temp file

	text, err := ocr.RecognizeText(imagePath)
	if err != nil {
		if a.window != nil {
			a.window.Show()
		}
		return "", fmt.Errorf("recognize text: %w", err)
	}

	// Show window and populate text
	a.showWindows()
	if text != "" {
		a.setClipboardText(text)
	}
	return text, nil
}

func (a *App) setClipboardText(text string) {
	if a.app != nil {
		a.app.Event.Emit("set-clipboard-text", text)
	}
}

func (a *App) showWindows() {
	if a.window != nil {
		a.window.Show()
		a.window.Focus()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Window & Clipboard
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) ToggleWindowVisibility() {
	text, err := clipboard.GetText(a.app)
	if err != nil {
		slog.Error("get clipboard", "error", err)
		return
	}
	a.showWindows()
	if text != "" {
		a.setClipboardText(text)
	}
}

func (a *App) GetAccessibilityPermission() bool {
	return hotkey.IsAccessibilityEnabled(false)
}

func (a *App) GetScreenRecordingPermission() bool {
	return screenshot.HasPermission()
}

func (a *App) RequestScreenRecordingPermission() {
	screenshot.RequestPermission()
}

func (a *App) GetVersion() string {
	return version
}

// ─────────────────────────────────────────────────────────────────────────────
// Provider Management (Legacy - for backward compatibility)
// ─────────────────────────────────────────────────────────────────────────────

// GetProviders returns legacy Provider list for backward compatibility.
// Deprecated: Use GetTranslationProfiles instead.
func (a *App) GetProviders() []types.Provider {
	// Build legacy providers from new format
	var providers []types.Provider
	for _, profile := range a.cfg.GetTranslationProfiles() {
		cred := a.cfg.GetCredential(profile.CredentialID)
		if cred == nil {
			continue
		}
		providers = append(providers, types.Provider{
			Name:            profile.Name,
			Type:            cred.Type,
			BaseURL:         cred.BaseURL,
			APIKey:          cred.APIKey,
			Model:           profile.Model,
			SystemPrompt:    profile.SystemPrompt,
			MaxTokens:       profile.MaxTokens,
			Temperature:     profile.Temperature,
			Active:          profile.Active,
			DisableThinking: profile.DisableThinking,
		})
	}
	return providers
}

// AddProvider adds a legacy provider by creating credential + profile.
// Deprecated: Use AddCredential + AddTranslationProfile instead.
func (a *App) AddProvider(p types.Provider) error {
	return a.cfg.AddProvider(p)
}

// UpdateProvider updates a legacy provider.
// Deprecated: Use UpdateTranslationProfile instead.
func (a *App) UpdateProvider(name string, p types.Provider) error {
	return a.cfg.UpdateProvider(name, p)
}

// RemoveProvider removes a legacy provider.
// Deprecated: Use RemoveTranslationProfile instead.
func (a *App) RemoveProvider(name string) error {
	return a.cfg.RemoveProvider(name)
}

// SetProviderActive sets a legacy provider as active.
// Deprecated: Use SetTranslationProfileActive instead.
func (a *App) SetProviderActive(name string) error {
	return a.cfg.SetProviderActive(name)
}

// GetActiveProvider returns the active provider in legacy format.
func (a *App) GetActiveProvider() *types.Provider {
	return a.cfg.GetActiveProviderCompat()
}

// ─────────────────────────────────────────────────────────────────────────────
// API Credential Management (New Architecture)
// ─────────────────────────────────────────────────────────────────────────────

// GetCredentials returns all API credentials.
func (a *App) GetCredentials() []types.APICredential {
	return a.cfg.GetCredentials()
}

// AddCredential adds a new API credential.
func (a *App) AddCredential(cred types.APICredential) error {
	return a.cfg.AddCredential(cred)
}

// UpdateCredential updates an existing credential.
func (a *App) UpdateCredential(id string, cred types.APICredential) error {
	return a.cfg.UpdateCredential(id, cred)
}

// RemoveCredential removes a credential by ID.
func (a *App) RemoveCredential(id string) error {
	return a.cfg.RemoveCredential(id)
}

// ─────────────────────────────────────────────────────────────────────────────
// Translation Profile Management (New Architecture)
// ─────────────────────────────────────────────────────────────────────────────

// GetTranslationProfiles returns all translation profiles.
func (a *App) GetTranslationProfiles() []types.TranslationProfile {
	return a.cfg.GetTranslationProfiles()
}

// GetActiveTranslationProfile returns the currently active translation profile.
func (a *App) GetActiveTranslationProfile() *types.TranslationProfile {
	return a.cfg.GetActiveTranslationProfile()
}

// AddTranslationProfile adds a new translation profile.
func (a *App) AddTranslationProfile(profile types.TranslationProfile) error {
	return a.cfg.AddTranslationProfile(profile)
}

// UpdateTranslationProfile updates an existing translation profile.
func (a *App) UpdateTranslationProfile(id string, profile types.TranslationProfile) error {
	return a.cfg.UpdateTranslationProfile(id, profile)
}

// RemoveTranslationProfile removes a translation profile by ID.
func (a *App) RemoveTranslationProfile(id string) error {
	return a.cfg.RemoveTranslationProfile(id)
}

// SetTranslationProfileActive sets a translation profile as active.
func (a *App) SetTranslationProfileActive(id string) error {
	return a.cfg.SetTranslationProfileActive(id)
}

// ─────────────────────────────────────────────────────────────────────────────
// Speech Configuration (New Architecture)
// ─────────────────────────────────────────────────────────────────────────────

// GetSpeechConfig returns the speech service configuration.
func (a *App) GetSpeechConfig() *types.SpeechConfig {
	return a.cfg.GetSpeechConfig()
}

// SetSpeechConfig sets the speech service configuration.
func (a *App) SetSpeechConfig(cfg types.SpeechConfig) error {
	if err := a.cfg.SetSpeechConfig(cfg); err != nil {
		return err
	}
	// Reinitialize STT registry with new config
	if a.sttRegistry != nil {
		a.sttRegistry.Close()
	}
	a.setupSTT()
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Language Settings
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) GetDefaultLanguages() map[string]string {
	return a.cfg.DefaultLanguages
}

func (a *App) SetDefaultLanguage(src, dst string) error {
	if a.cfg.DefaultLanguages == nil {
		a.cfg.DefaultLanguages = make(map[string]string)
	}
	a.cfg.DefaultLanguages[src] = dst
	return a.cfg.Save()
}

func (a *App) DetectLanguage(text string) types.DetectResult {
	code, name := langdetect.Detect(text)

	target := "en"
	if code != "auto" && a.cfg.DefaultLanguages != nil {
		if t, ok := a.cfg.DefaultLanguages[code]; ok {
			target = t
		}
	}

	return types.DetectResult{
		Code:          code,
		Name:          name,
		DefaultTarget: target,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Translation
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) TranslateWithLLM(req types.TranslateRequest) (types.TranslateResult, error) {
	provider := a.GetActiveProvider()
	if provider == nil {
		return types.TranslateResult{}, fmt.Errorf("no active provider configured")
	}

	cacheKey := a.translationCacheKey(provider, req)

	// Check cache first.
	if result, ok := a.getCachedTranslation(cacheKey); ok {
		return result, nil
	}

	// Call LLM API.
	text, usage, err := a.callLLM(provider, req)
	if err != nil {
		return types.TranslateResult{}, fmt.Errorf("translate %q: %w", truncate(req.Text, 32), err)
	}

	// Store result in cache (best effort).
	a.cacheTranslation(cacheKey, text, usage)

	return types.TranslateResult{Text: text, Usage: usage}, nil
}

// translationCacheKey generates a cache key for the translation request.
func (a *App) translationCacheKey(p *types.Provider, req types.TranslateRequest) string {
	return cache.GenerateKey(p.Name, p.Model, req.SourceLang, req.TargetLang, req.Text)
}

// getCachedTranslation retrieves a cached translation if available.
func (a *App) getCachedTranslation(key string) (types.TranslateResult, bool) {
	if a.cache == nil {
		return types.TranslateResult{}, false
	}

	entry, found := a.cache.Get(key)
	if !found {
		return types.TranslateResult{}, false
	}

	return types.TranslateResult{
		Text: entry.Text,
		Usage: types.Usage{
			PromptTokens:     entry.Usage.PromptTokens,
			CompletionTokens: entry.Usage.CompletionTokens,
			TotalTokens:      entry.Usage.TotalTokens,
			CacheHit:         true,
		},
	}, true
}

// cacheTranslation stores a translation result in the cache.
func (a *App) cacheTranslation(key, text string, usage types.Usage) {
	if a.cache == nil {
		return
	}

	entry := &cache.Entry{
		Text: text,
		Usage: cache.Usage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		},
		CreatedAt: time.Now(),
	}

	if err := a.cache.Set(key, entry, cache.DefaultTTL); err != nil {
		slog.Warn("cache translation", "error", err)
	}
}

// callLLM invokes the LLM API to perform translation.
func (a *App) callLLM(p *types.Provider, req types.TranslateRequest) (string, types.Usage, error) {
	client := llm.NewClient(p)

	content := fmt.Sprintf(
		"please translate the following text from %s to %s:\n\n%s",
		req.SourceLang, req.TargetLang, req.Text,
	)

	if req.Context != "" {
		content = fmt.Sprintf(
			"Context (previous sentences): %s\n\n%s",
			req.Context, content,
		)
	}

	messages := []llm.Message{
		{Role: "system", Content: p.SystemPrompt},
		{Role: "user", Content: content},
	}

	return client.Complete(messages)
}

// truncate shortens a string for logging purposes.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ─────────────────────────────────────────────────────────────────────────────
// Main Entry
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.Info("starting app", "version", version, "commit", commit, "date", date)
	appService := NewApp()

	app := application.New(application.Options{
		Name:        "Transy",
		Description: "AI-Powered Translation Tool",
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			// Don't quit when all windows are closed (we have a system tray)
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	// Create main window
	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Transy",
		Width:  1024,
		Height: 768,
		URL:    "/",
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInsetUnified,
			InvisibleTitleBarHeight: 38,
		},
		DevToolsEnabled: true,
	})

	// Intercept window close: hide instead of destroy so tray can reopen
	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel() // Prevent actual close
		mainWindow.Hide()
	})

	// Initialize service with app and window references
	appService.Init(app, mainWindow)

	// Setup system tray
	systemTray := app.SystemTray.New()

	// Use custom tray icon
	// using SetIcon to render original colors instead of template (monochrome mask)
	systemTray.SetIcon(trayIconBytes)

	// Create tray menu
	trayMenu := app.NewMenu()
	trayMenu.Add("显示窗口").OnClick(func(ctx *application.Context) {
		appService.showWindows()
	})
	trayMenu.Add("OCR 翻译").
		SetAccelerator("CmdOrCtrl+Shift+O").
		OnClick(func(ctx *application.Context) {
			go func() {
				if _, err := appService.TakeScreenshotAndOCR(); err != nil {
					slog.Error("ocr from tray", "error", err)
				}
			}()
		})

	// Provider submenu with radio buttons
	providerMenu := trayMenu.AddSubmenu("翻译服务")
	providers := appService.GetProviders()
	for _, p := range providers {
		provider := p // Capture loop variable
		providerMenu.AddRadio(provider.Name, provider.Active).OnClick(func(ctx *application.Context) {
			if err := appService.SetProviderActive(provider.Name); err != nil {
				slog.Error("set provider active", "error", err)
			}
		})
	}

	trayMenu.AddSeparator()
	trayMenu.Add("退出").
		SetAccelerator("CmdOrCtrl+Q").
		OnClick(func(ctx *application.Context) {
			appService.Shutdown()
			app.Quit()
		})

	systemTray.SetMenu(trayMenu)

	// Run application
	if err := app.Run(); err != nil {
		slog.Error("run app", "error", err)
	}
}
