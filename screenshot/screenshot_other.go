//go:build !darwin

package screenshot

// HasPermission checks if the app has screen recording permission.
func HasPermission() bool {
	return false
}

// RequestPermission requests screen recording permission from the system.
func RequestPermission() {}

// CaptureInteractive launches the interactive screenshot tool and saves the image to a temp file.
// Returns the path to the saved image file.
func CaptureInteractive() (string, error) {
	return "", nil
}
