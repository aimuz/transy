//go:build darwin

package audiocapture

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreMedia -framework CoreAudio -framework Foundation -framework AVFoundation

#include <stdlib.h>

// Declaration of the Objective-C functions implemented in capture_darwin.m
extern int startAudioCaptureWithCallback(void* goContext, int targetSampleRate);
extern void stopAudioCapture(void);
extern int isAudioCapturing(void);
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"
)

// Global registry to map context pointers back to Go callbacks
var (
	callbackRegistry   = make(map[uintptr]func([]float32))
	callbackRegistryMu sync.RWMutex
	callbackCounter    uintptr
)

//export goAudioCallback
func goAudioCallback(context unsafe.Pointer, samples *C.float, count C.int) {
	ptr := uintptr(context)

	callbackRegistryMu.RLock()
	callback, ok := callbackRegistry[ptr]
	callbackRegistryMu.RUnlock()

	if !ok || callback == nil {
		return
	}

	// Convert C array to Go slice
	n := int(count)
	if n <= 0 {
		return
	}

	goSamples := make([]float32, n)
	cSamples := (*[1 << 28]C.float)(unsafe.Pointer(samples))[:n:n]
	for i := 0; i < n; i++ {
		goSamples[i] = float32(cSamples[i])
	}

	callback(goSamples)
}

// darwinCaptureImpl is the macOS implementation using ScreenCaptureKit.
type darwinCaptureImpl struct {
	contextPtr uintptr
	capturing  bool
	mu         sync.Mutex
}

func newCaptureImpl() (captureImpl, error) {
	return &darwinCaptureImpl{}, nil
}

func (d *darwinCaptureImpl) start(sampleRate int, callback func([]float32)) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.capturing {
		return ErrAlreadyCapturing
	}

	// Register callback
	callbackRegistryMu.Lock()
	callbackCounter++
	d.contextPtr = callbackCounter
	callbackRegistry[d.contextPtr] = callback
	callbackRegistryMu.Unlock()

	// Start capture
	result := C.startAudioCaptureWithCallback(unsafe.Pointer(d.contextPtr), C.int(sampleRate))
	if result != 0 {
		// Clean up callback
		callbackRegistryMu.Lock()
		delete(callbackRegistry, d.contextPtr)
		callbackRegistryMu.Unlock()

		switch result {
		case -1:
			return fmt.Errorf("failed to get shareable content (screen recording permission required)")
		case -2:
			return fmt.Errorf("no displays available")
		case -3:
			return fmt.Errorf("failed to add stream output")
		case -4:
			return fmt.Errorf("failed to start capture")
		case -100:
			return fmt.Errorf("macOS 12.3 or later required for ScreenCaptureKit")
		default:
			return fmt.Errorf("unknown error: %d", result)
		}
	}

	d.capturing = true
	return nil
}

func (d *darwinCaptureImpl) stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.capturing {
		return nil
	}

	C.stopAudioCapture()

	// Clean up callback
	callbackRegistryMu.Lock()
	delete(callbackRegistry, d.contextPtr)
	callbackRegistryMu.Unlock()

	d.capturing = false
	return nil
}

func (d *darwinCaptureImpl) isCapturing() bool {
	return C.isAudioCapturing() == 1
}
