package main

import (
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
	"go.aimuz.me/transy/llm"
	"go.aimuz.me/transy/ocr"
	"go.aimuz.me/transy/screenshot"
)

//go:embed all:frontend/dist
var assets embed.FS

// App is the main application service bound to Wails.
type App struct {
	app    *application.App
	window application.Window
	cfg    *config.Config
	hotkey *hotkey.HotkeyManager
	cache  *cache.Cache
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

	a.setupHotkey()
}

// Shutdown cleans up resources.
func (a *App) Shutdown() {
	if a.hotkey != nil {
		a.hotkey.Stop()
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

// ─────────────────────────────────────────────────────────────────────────────
// Provider Management (Delegated to Config)
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) GetProviders() []types.Provider {
	return a.cfg.Providers
}

func (a *App) AddProvider(p types.Provider) error {
	return a.cfg.AddProvider(p)
}

func (a *App) UpdateProvider(name string, p types.Provider) error {
	return a.cfg.UpdateProvider(name, p)
}

func (a *App) RemoveProvider(name string) error {
	return a.cfg.RemoveProvider(name)
}

func (a *App) SetProviderActive(name string) error {
	return a.cfg.SetProviderActive(name)
}

func (a *App) GetActiveProvider() *types.Provider {
	return a.cfg.GetActiveProvider()
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

	messages := []llm.Message{
		{Role: "system", Content: p.SystemPrompt},
		{Role: "user", Content: fmt.Sprintf(
			"please translate the following text from %s to %s:\n\n%s",
			req.SourceLang, req.TargetLang, req.Text,
		)},
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
