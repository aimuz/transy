package screenshot

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework Foundation
#import <CoreGraphics/CoreGraphics.h>
#import <Foundation/Foundation.h>

bool hasScreenRecordingPermission() {
    if (@available(macOS 11.0, *)) {
        return CGPreflightScreenCaptureAccess();
    }
    // Fallback for macOS 10.15
    // Note: On 10.15, there isn't a direct preflight API.
    // We can try to capture a tiny bit of screen to check.
    return true; // Assume true or implement a check
}

void requestScreenRecordingPermission() {
    if (@available(macOS 11.0, *)) {
        CGRequestScreenCaptureAccess();
    }
}
*/
import "C"
import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// HasPermission checks if the app has screen recording permission.
func HasPermission() bool {
	return bool(C.hasScreenRecordingPermission())
}

// RequestPermission requests screen recording permission from the system.
func RequestPermission() {
	C.requestScreenRecordingPermission()
}

// CaptureInteractive launches the interactive screenshot tool and saves the image to a temp file.
// Returns the path to the saved image file.
func CaptureInteractive() (string, error) {
	// Create a temporary file path
	tmpDir := os.TempDir()
	fileName := fmt.Sprintf("transy_screenshot_%d.png", time.Now().UnixNano())
	filePath := filepath.Join(tmpDir, fileName)

	// Command: screencapture -i <path>
	// -i: capture interactively (selection)
	// -x: do not play sound
	cmd := exec.Command("screencapture", "-i", "-x", filePath)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("screencapture failed: %w", err)
	}

	// Check if file exists (user might have cancelled)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("screenshot cancelled or failed to save")
	}

	return filePath, nil
}
