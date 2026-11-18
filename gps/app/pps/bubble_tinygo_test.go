//go:build tinygo

package pps

import "testing"

// runBubble skips: TinyGo does not implement testing/synctest.
func runBubble(t *testing.T, _ func(*testing.T)) {
	t.Skip("TinyGo does not implement testing/synctest")
}
