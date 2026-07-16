package main

/*
#cgo LDFLAGS: -framework CoreServices
#include <stdlib.h>
#include <CoreServices/CoreServices.h>

static OSStatus openURL(const char *s, size_t len) {
	CFURLRef url = CFURLCreateWithBytes(NULL, (const UInt8 *)s, len, kCFStringEncodingASCII, NULL);
	if (url == NULL) {
		return coreFoundationUnknownErr;
	}
	OSStatus st = LSOpenCFURLRef(url, NULL);
	CFRelease(url);
	return st;
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func hasGraphicalSession() bool {
	return true
}

// launchBrowser hands the URL to LaunchServices in-process rather than
// spawning open(1): setuid ps exposes other users' argv even on macOS,
// so the token-bearing URL must never appear on a command line.
func launchBrowser(url string) error {
	s := C.CString(url)
	defer C.free(unsafe.Pointer(s))
	if st := C.openURL(s, C.size_t(len(url))); st != 0 {
		return fmt.Errorf("LSOpenCFURLRef: OSStatus %d", st)
	}
	return nil
}

// launchBrowserLeaksURL reports whether launchBrowser exposes the URL to
// other local users. False on macOS: LSOpenCFURLRef runs in-process and
// the URL reaches no command line.
const launchBrowserLeaksURL = false
