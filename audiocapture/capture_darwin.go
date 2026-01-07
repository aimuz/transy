//go:build darwin

package audiocapture

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreMedia -framework CoreAudio -framework Foundation -framework AVFoundation

#include <stdlib.h>

extern int startAudioCapture(int targetSampleRate, char** errOut);
extern void stopAudioCapture(void);
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// Global handler for CGO callback. Only one capture at a time.
var (
	globalHandler   AudioHandler
	globalHandlerMu sync.RWMutex
)

//export goAudioCallback
func goAudioCallback(samples *C.float, count C.int) {
	n := int(count)
	if n <= 0 {
		return
	}

	globalHandlerMu.RLock()
	h := globalHandler
	globalHandlerMu.RUnlock()

	if h == nil {
		return
	}

	// Convert C array to Go slice without extra allocation.
	// Safe because we process samples before this function returns.
	goSamples := unsafe.Slice((*float32)(unsafe.Pointer(samples)), n)
	h(goSamples)
}

// capturer is the macOS implementation using ScreenCaptureKit.
type capturer struct {
	sampleRate int
	mu         sync.Mutex
	running    bool
}

// New creates a Capturer for macOS.
func New(sampleRate int) (Capturer, error) {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	return &capturer{sampleRate: sampleRate}, nil
}

func (c *capturer) Start(handler AudioHandler) error {
	if handler == nil {
		return errors.New("audiocapture: nil handler")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return ErrRunning
	}

	// Set global handler before starting capture.
	globalHandlerMu.Lock()
	globalHandler = handler
	globalHandlerMu.Unlock()

	var errStr *C.char
	result := C.startAudioCapture(C.int(c.sampleRate), &errStr)
	if result != 0 {
		globalHandlerMu.Lock()
		globalHandler = nil
		globalHandlerMu.Unlock()

		if errStr != nil {
			err := errors.New(C.GoString(errStr))
			C.free(unsafe.Pointer(errStr))
			return err
		}
		return errors.New("audiocapture: unknown error")
	}

	c.running = true
	return nil
}

func (c *capturer) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	C.stopAudioCapture()

	globalHandlerMu.Lock()
	globalHandler = nil
	globalHandlerMu.Unlock()

	c.running = false
	return nil
}
