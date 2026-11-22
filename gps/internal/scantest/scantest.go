package scantest

import (
	"math/rand/v2"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func FindPacket(pf gpsprot.PacketFormat, buf []byte) (startPos, endPos int, ok bool) {
	state := gpsprot.ScanStateSync
	startPos = -1
	endPos = -1
	for i := range buf {
		packetLen := 0
		if startPos >= 0 {
			packetLen = i - startPos
		}
		state = pf.Next(state, buf, i, packetLen)
		if state == 0 {
			if startPos >= 0 {
				endPos = i
				ok = false
				break
			}
		} else {
			if startPos < 0 {
				startPos = i
			}
			if pf.IsFinal(state) {
				ok = true
				endPos = i + 1
				break
			}
		}
	}
	return
}

func InsertRandomPrefix(data string, except byte) ([]byte, int) {
	nRandom := rand.IntN(100)
	buf := make([]byte, nRandom+len(data))
	for i := range nRandom {
		b := byte(rand.IntN(256))
		if b == except {
			b = 0xFF
		}
		buf[i] = b
	}
	copy(buf[nRandom:], data)
	return buf, nRandom
}

func TrimNMEA(data string) string {
	trimmed := data
	if len(data) > 0 && data[len(data)-1] == '\n' {
		trimmed = data[0 : len(data)-1]
	}
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\r' {
		trimmed = trimmed[0 : len(trimmed)-1]
	}
	return trimmed
}

// IsValidPacketBetween tests if a packet is valid when embedded in a larger buffer
// start and end are indices in buf where the packet data is located
func IsValidPacketBetween(pf gpsprot.PacketFormat, buf []byte, start, end int) bool {
	if start < 0 || end > len(buf) || start >= end {
		return false
	}

	state := gpsprot.ScanStateSync
	packetLen := 0

	for i := start; i < end; i++ {
		state = pf.Next(state, buf, i, packetLen)
		if state == gpsprot.ScanStateSync {
			return false
		}
		packetLen++
	}

	return pf.IsFinal(state)
}
