// Package osthread changes the scheduling settings of an OS thread.
//
// Each function acts on the calling OS thread, so the caller must first lock
// its goroutine to that thread with runtime.LockOSThread. The settings stay
// with the thread. A goroutine that changes them and exits without unlocking
// the thread makes the runtime terminate the thread, so they never reach
// other goroutines.
//
// The package is implemented for Linux. Elsewhere each function returns an
// error wrapping errors.ErrUnsupported.
package osthread
