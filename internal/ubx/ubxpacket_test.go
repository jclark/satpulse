package ubx

import (
	"bytes"
	"testing"

	"github.com/jclark/satpulse/internal/ubx/bin"
)

func TestPacketChecksum(t *testing.T) {
	pkt := bin.PollCfgTp5(0)
	if !bytes.Equal(PacketFormat.ComputeChecksum(pkt), PacketFormat.ExtractChecksum(pkt)) {
		t.Fatalf("checksum of generated UBX packet did not have expected value")
	}
}
