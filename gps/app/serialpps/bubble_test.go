//go:build !windows

package serialpps

import (
	"testing"
	"testing/synctest"
)

// runBubble runs f in a synctest bubble, so the polling loop's sleeps and its
// clock readings both come from the bubble's fake clock.
func runBubble(t *testing.T, f func(*testing.T)) {
	synctest.Test(t, f)
}
