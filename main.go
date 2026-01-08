package main

import (
	"embed"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"go.aimuz.me/transy/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.Info("starting app", "version", version, "commit", commit, "date", date)

	service := app.New(version)

	wailsApp := application.New(application.Options{
		Name:        "Transy",
		Description: "AI-Powered Translation Tool",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
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

	// Hide instead of close for tray reopen
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
	})

	service.Init(wailsApp, window)

	// Setup system tray
	setupSystemTray(wailsApp, service)

	if err := wailsApp.Run(); err != nil {
		slog.Error("run app", "error", err)
	}
}

func setupSystemTray(wailsApp *application.App, service *app.Service) {
	systemTray := wailsApp.SystemTray.New()
	systemTray.SetIcon(trayIconBytes)

	trayMenu := wailsApp.NewMenu()
	trayMenu.Add("显示窗口").OnClick(func(ctx *application.Context) {
		service.ToggleWindowVisibility()
	})
	trayMenu.Add("OCR 翻译").
		SetAccelerator("CmdOrCtrl+Shift+O").
		OnClick(func(ctx *application.Context) {
			go func() {
				if _, err := service.TakeScreenshotAndOCR(); err != nil {
					slog.Error("ocr from tray", "error", err)
				}
			}()
		})

	// Provider submenu
	providerMenu := trayMenu.AddSubmenu("翻译服务")
	profiles := service.GetTranslationProfiles()
	for _, p := range profiles {
		profile := p // capture
		providerMenu.AddRadio(profile.Name, profile.Active).OnClick(func(ctx *application.Context) {
			if err := service.SetTranslationProfileActive(profile.ID); err != nil {
				slog.Error("set profile active", "error", err)
			}
		})
	}

	trayMenu.AddSeparator()
	trayMenu.Add("退出").
		SetAccelerator("CmdOrCtrl+Q").
		OnClick(func(ctx *application.Context) {
			service.Shutdown()
			wailsApp.Quit()
		})

	systemTray.SetMenu(trayMenu)
}
