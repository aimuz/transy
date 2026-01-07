package ocr

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=10.15
#cgo LDFLAGS: -framework Vision -framework Foundation -framework CoreImage

#include <stdlib.h>

// Declaration of the Objective-C function implemented in ocr_darwin.m
extern char* recognizeText(const char* imagePath);
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// RecognizeText performs OCR on the image at the given path.
// It returns the recognized text or an error.
func RecognizeText(imagePath string) (string, error) {
	cPath := C.CString(imagePath)
	defer C.free(unsafe.Pointer(cPath))

	cResult := C.recognizeText(cPath)
	if cResult == nil {
		return "", fmt.Errorf("OCR failed to recognize text or load image")
	}
	defer C.free(unsafe.Pointer(cResult))

	return C.GoString(cResult), nil
}
