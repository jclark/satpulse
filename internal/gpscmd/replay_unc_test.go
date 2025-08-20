package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsio"
)

var uncReplayFiles = []string{
	"unc/show",
}

func TestReplayUNC(t *testing.T) {
	for _, filename := range uncReplayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, filename, uncPacketsEqual)
		})
	}
}

func uncPacketsEqual(t *testing.T, msgID string, actual []byte, expected gpsio.PacketLogEntry) bool {
	actualStr := string(actual)
	expectedStr := expected.Data()

	// First check if they're exactly equal
	if actualStr == expectedStr {
		return true
	}

	// UNC packets are simpler than UBX - they're mostly ASCII commands
	// For ASCII packets, we can compare them as strings directly
	// For binary packets, we would need more sophisticated comparison, but
	// the test data appears to be ASCII commands
	
	t.Errorf("%s: packets differ: actual %q, expected %q", msgID, actualStr, expectedStr)
	return false
}