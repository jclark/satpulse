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

func TestDecodeAsciiChunkedCount(t *testing.T) {
	tests := []struct {
		name      string
		fields    []string
		expectErr bool
	}{
		{
			name:   "count matches records",
			fields: []string{"1.5", "1.2", "0.8", "0.9", "0.7", "5.0", "2", "3", "7"},
		},
		{
			name:      "count larger than records",
			fields:    []string{"1.5", "1.2", "0.8", "0.9", "0.7", "5.0", "3", "3", "7"},
			expectErr: true,
		},
		{
			name:      "count smaller than records",
			fields:    []string{"1.5", "1.2", "0.8", "0.9", "0.7", "5.0", "1", "3", "7"},
			expectErr: true,
		},
		{
			name:      "truncated initial chunk",
			fields:    []string{"1.5", "1.2"},
			expectErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m PsrDop
			err := DecodeAsciiChunked(tc.fields, &m, "PSRDOPA")
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", m)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
