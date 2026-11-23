// Package osthread tunes the operating system thread that the calling
// goroutine runs on, for code that polls with tight timing: its CPU
// affinity, its scheduling policy, its timer slack, and sleeps made by the
// thread itself rather than through the Go runtime's timers.
//
// Each function acts on the calling OS thread, so the caller must first lock
// its goroutine to that thread with runtime.LockOSThread. The settings stay
// with the thread. A goroutine that changes them and exits without unlocking
// the thread makes the runtime terminate the thread, so they never reach
// other goroutines.
//
// The package is implemented for Linux. Elsewhere SetAffinity and SetFIFO
// return an error wrapping errors.ErrUnsupported, TightenTimerSlack does
// nothing, and SleepUntil does not sleep.
package osthread
