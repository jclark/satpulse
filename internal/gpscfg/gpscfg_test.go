package gpscfg

import (
	"log/slog"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/rtcm"
	"github.com/jclark/satpulse/internal/ubx"
)

func TestNativeOnlyDetection(t *testing.T) {
	// Create msgHandler with real packet processors
	mh := msgHandler{
		lg:          slog.Default(),
		packetProcs: gpsreg.CreatePacketProcessors(nil),
		msgCount:    make(map[gpsprot.Tag]int),
	}

	// Simulate receiving messages from different protocols
	mh.msgCount[ubx.Tag] = 5
	mh.msgCount[nmea.Tag] = 3
	mh.msgCount[rtcm.Tag] = 7

	// Test suitableMessageCount - should only count non-native-only protocols
	suitable := mh.suitableMessageCount()
	if suitable != 8 {
		t.Errorf("suitableMessageCount() = %d, want 8", suitable)
	}

	// Test nativeOnlyTags - should only return RTCM
	nativeOnly := mh.nativeOnlyTags()
	if len(nativeOnly) != 1 || nativeOnly[0] != "RTCM" {
		t.Errorf("nativeOnlyTags() = %v, want [RTCM]", nativeOnly)
	}
}