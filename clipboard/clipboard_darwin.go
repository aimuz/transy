//go:build darwin

package clipboard

import (
	"errors"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// #cgo CFLAGS: -x objective-c
// #cgo LDFLAGS: -framework Cocoa
// #import <Cocoa/Cocoa.h>
// const char* getClipboardContent() {
//     NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
//     NSString *string = [pasteboard stringForType:NSPasteboardTypeString];
//     return [string UTF8String];
// }
import "C"

var clipboardLock sync.RWMutex

func getClipboardContent(_ *application.App) (string, error) {
	clipboardLock.RLock()
	defer clipboardLock.RUnlock()

	cstr := C.getClipboardContent()
	if cstr == nil {
		return "", errors.New("failed to get clipboard content")
	}
	return C.GoString(cstr), nil
}
