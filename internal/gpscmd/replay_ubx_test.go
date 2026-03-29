package gpscmd

import (
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

var ubxSchema = ubxcfgval.NewSchemaWithMsgout(ubxcfgval.GetDfltSchema())

var replayFiles = []string{
	"f9p-noop",
	"f9p-signal",
	"f9p-reload",
	"f9p-binary",
	"f9p-raw-out",
	"f9p-rtcm-out",
	"f9p-pvt-out",
	"f9p-tmode",
	"f9t-noop",
	"f9t-rtcm-out",
	"f9t-signal",
	"f9t-tmode",
	"m8-noop",
	"m8-speed",
	"m8-signal",
	"m8-tmode",
	"f10t-signal",
	"f10t-noop",
	"f10t-tmode",
	"f10t-pps",
	"m8t-binary",
	"m8t-noop",
	"m8t-pps",
	"m8t-signal",
	"m8t-speed",
	"m8t-tmode",
	"x20p-noop",
	"x20p-signal",
	"x20p-tmode",
	"x20p-pps",
	"x20p-speed",
	"x20p-binary",
	"x20p-rtcm-out",
	"x20p-raw-out",
	"x20p-pvt-out",
	"x20p-reload",
}

func TestReplayUBX(t *testing.T) {
	for _, filename := range replayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, filename, ubxPacketsEqual)
		})
	}
}

func HideTestReplayBug(t *testing.T) {
	testReplayFile(t, "bug", ubxPacketsEqual)
}

// ubxPacketsEqual returns (equal, updatable).
// updatable is true when the mismatch is in CFG-VALSET/CFG-VALGET content
// and is safe to auto-update in golden files.
func ubxPacketsEqual(t *testing.T, msgID string, actual []byte, expected gpsio.PacketLogEntry) (bool, bool) {
	actualStr := string(actual)
	expectedStr := expected.Data()

	// First check if they're exactly equal
	if actualStr == expectedStr {
		return true, false
	}

	// If not, check special cases for messages that might have reordered data
	switch msgID {
	case "CFG-VALSET":
		eq := valsetPacketsEqual(t, actualStr, expectedStr)
		return eq, !eq
	case "CFG-VALGET":
		// For output packets (which these always are), cfgData contains keys
		eq := valgetPacketsEqual(t, actualStr, expectedStr)
		return eq, !eq
	default:
		// Try to parse both messages to provide better error details
		actualMsg, actualErr := ubxbin.ParseMsg(actualStr)
		expectedMsg, expectedErr := ubxbin.ParseMsg(expectedStr)

		if actualErr != nil {
			t.Errorf("%s: failed to parse actual packet: %v", msgID, actualErr)
		}
		if expectedErr != nil {
			t.Errorf("%s: failed to parse expected packet: %v", msgID, expectedErr)
		}

		if actualErr == nil && expectedErr == nil {
			t.Errorf("%s: packets differ: actual %+v, expected %+v", msgID, actualMsg, expectedMsg)
		} else if actualErr == nil || expectedErr == nil {
			t.Errorf("%s: packet content differs (length actual=%d, expected=%d)", msgID, len(actualStr), len(expectedStr))
		}

		return false, false
	}
}

func valsetPacketsEqual(t *testing.T, actual, expected string) bool {
	// Parse both packets
	actualMsg, err := ubxbin.ParseMsg(actual)
	if err != nil {
		t.Errorf("CFG-VALSET: failed to parse actual packet: %v", err)
		return false
	}
	expectedMsg, err := ubxbin.ParseMsg(expected)
	if err != nil {
		t.Errorf("CFG-VALSET: failed to parse expected packet: %v", err)
		return false
	}

	actualValset, ok := actualMsg.(*ubxbin.CfgValset)
	if !ok {
		return false
	}
	expectedValset, ok := expectedMsg.(*ubxbin.CfgValset)
	if !ok {
		t.Errorf("expected %s, but got CFG-VALSET", expectedMsg.ID().String())
		return false
	}

	// Compare fixed fields
	if actualValset.CfgValsetFixed != expectedValset.CfgValsetFixed {
		t.Errorf("CFG-VALSET: fixed fields differ: actual %+v, expected %+v",
			actualValset.CfgValsetFixed, expectedValset.CfgValsetFixed)
		return false
	}

	// Parse and compare configuration items
	actualKeys, actualValues, err := ubxSchema.UnmarshalItemsFlat(actualValset.CfgData)
	if err != nil {
		t.Errorf("CFG-VALSET: failed to unmarshal actual items: %v", err)
		return false
	}

	expectedKeys, expectedValues, err := ubxSchema.UnmarshalItemsFlat(expectedValset.CfgData)
	if err != nil {
		t.Errorf("CFG-VALSET: failed to unmarshal expected items: %v", err)
		return false
	}

	// Create maps for comparison
	actualMap := make(map[string]any)
	expectedMap := make(map[string]any)

	for i, key := range actualKeys {
		actualMap[key] = actualValues[i]
	}
	for i, key := range expectedKeys {
		expectedMap[key] = expectedValues[i]
	}

	// Check each expected key
	equal := true
	for key, expectedVal := range expectedMap {
		if actualVal, exists := actualMap[key]; !exists {
			t.Errorf("CFG-VALSET: missing key: %s (expected value: %v)", key, expectedVal)
			equal = false
		} else if actualVal != expectedVal {
			t.Errorf("CFG-VALSET: key %s: actual %v, expected %v", key, actualVal, expectedVal)
			equal = false
		}
	}

	// Check for unexpected keys
	for key, actualVal := range actualMap {
		if _, exists := expectedMap[key]; !exists {
			t.Errorf("CFG-VALSET: unexpected key: %s (actual value: %v)", key, actualVal)
			equal = false
		}
	}

	return equal
}

func valgetPacketsEqual(t *testing.T, actual, expected string) bool {
	// Parse both packets
	actualMsg, err := ubxbin.ParseMsg(actual)
	if err != nil {
		t.Errorf("CFG-VALGET: failed to parse actual packet: %v", err)
		return false
	}
	expectedMsg, err := ubxbin.ParseMsg(expected)
	if err != nil {
		t.Errorf("CFG-VALGET: failed to parse expected packet: %v", err)
		return false
	}

	actualValget, ok := actualMsg.(*ubxbin.CfgValget)
	if !ok {
		return false
	}
	expectedValget, ok := expectedMsg.(*ubxbin.CfgValget)
	if !ok {
		t.Errorf("expected %s, but got CFG-VALGET", expectedMsg.ID().String())
		return false
	}

	// Compare fixed fields
	if actualValget.CfgValgetFixed != expectedValget.CfgValgetFixed {
		t.Errorf("CFG-VALGET: fixed fields differ: actual %+v, expected %+v",
			actualValget.CfgValgetFixed, expectedValget.CfgValgetFixed)
		return false
	}

	// For output packets, cfgData contains keys only
	actualKeys, err := ubxSchema.UnmarshalKeysFlat(actualValget.CfgData)
	if err != nil {
		t.Errorf("CFG-VALGET: failed to unmarshal actual keys: %v", err)
		return false
	}

	expectedKeys, err := ubxSchema.UnmarshalKeysFlat(expectedValget.CfgData)
	if err != nil {
		t.Errorf("CFG-VALGET: failed to unmarshal expected keys: %v", err)
		return false
	}

	// Create maps for comparison (to ignore order)
	actualMap := make(map[string]bool)
	expectedMap := make(map[string]bool)

	for _, key := range actualKeys {
		actualMap[key] = true
	}
	for _, key := range expectedKeys {
		expectedMap[key] = true
	}

	// Check each expected key
	equal := true
	for key := range expectedMap {
		if !actualMap[key] {
			t.Errorf("CFG-VALGET: missing expected key: %s", key)
			equal = false
		}
	}

	// Check for unexpected keys
	for key := range actualMap {
		if !expectedMap[key] {
			t.Errorf("CFG-VALGET: unexpected extra key: %s", key)
			equal = false
		}
	}

	return equal
}
