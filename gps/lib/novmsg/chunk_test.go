package novmsg

import (
	"encoding/hex"
	"runtime"
	"testing"
)

func TestChunkedHugeCount(t *testing.T) {
	b, err := hex.DecodeString(psrDopTests[0].hex)
	if err != nil {
		t.Fatalf("decoding hex: %v", err)
	}
	msg, err := ParseBinMsg(b)
	if err != nil {
		t.Fatalf("parsing test message: %v", err)
	}
	// Serializing writes NumPRNs as given but only the PRNs present,
	// giving a checksum-valid message whose count is wrong.
	msg.Body.(*PsrDop).NumPRNs = 0xffffffff
	b, err = SerializeBinMsg(msg)
	if err != nil {
		t.Fatalf("serializing: %v", err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = ParseBinMsg(b)
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatalf("expected error")
	}
	if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
		t.Errorf("allocated %d bytes", n)
	}
}
