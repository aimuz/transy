//go:build !darwin

package audiocapture

// New returns ErrUnsupported on non-macOS platforms.
func New(sampleRate int) (Capturer, error) {
	return nil, ErrUnsupported
}
