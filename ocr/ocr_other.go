//go:build !darwin

package ocr

// RecognizeText performs OCR on the image at the given path.
// It returns the recognized text or an error.
func RecognizeText(imagePath string) (string, error) {
	return "", nil
}
