package screenshot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

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
