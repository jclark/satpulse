package ubx

import (
	"fmt"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/ptime"
)

type trustedTimePacketBuilder struct{}

func (trustedTimePacketBuilder) TrustedTimePacket(est *gpsprot.TimeEstimate, now time.Time) ([]byte, error) {
	if est == nil {
		panic("trusted time estimate is nil")
	}
	if est.EstimatedTime.IsZero() {
		panic("trusted time estimate has zero estimated time")
	}
	mga, err := mgaTime(est, now)
	if err != nil {
		return nil, err
	}
	return ubxbin.Serialize(mga)
}

// mgaTime creates a trusted UBX-MGA-INI-TIMEUTC message from a gpsprot.TimeEstimate.
func mgaTime(ta *gpsprot.TimeEstimate, now time.Time) (*ubxbin.MgaIni, error) {
	initialEst := ta.EstimatedTime.UTC()
	t := initialEst // need this for leap second handling below
	if !ta.TimeOfEstimate.IsZero() {
		t = t.Add(now.Sub(ta.TimeOfEstimate))
	}
	acc := ta.Accuracy
	const dfltAccuracy = time.Second * 10 // comfortably less than 15 seconds that ublox needs for OSNMA
	if acc == 0 {
		acc = dfltAccuracy
	} else {
		// adjust to account for the time it takes to send the message
		// we don't have the baud rate available here, so assume worst case of 4800 baud
		// 30 byte message at 4800 baud is about 62.5ms
		// add a bit for latency and safety
		acc += time.Millisecond * 100
	}
	tAccS := acc / time.Second
	tAccNs := uint32(acc - tAccS*time.Second)

	leapSecs := ubxbin.MgaIniTimeUTCLeapSecsUnknown
	if ta.LeapSecond.UTCOffset > 0 {
		off := int(ta.LeapSecond.UTCOffset) - ptime.TAIMinusGPS
		// If in adjusting the time estimate we crossed a day boundary at which a leap second would have been applied,
		// then we need to adjust the leap second offset.
		if !ta.TimeOfEstimate.IsZero() && ta.LeapSecond.LeapTonight != ptime.LeapSecondNone {
			timeUntilLeap := initialEst.Truncate(time.Hour*24).AddDate(0, 0, 1).Sub(initialEst)
			if ta.LeapSecond.LeapTonight == ptime.LeapSecondNegative {
				timeUntilLeap -= time.Second
			}
			if t.Sub(ta.TimeOfEstimate) > timeUntilLeap {
				// too difficult; give up
				// XXX better approach would be to use MgaIniTimeGNSS when we know the leap seconds
				// but this is so vanishingly unlikely that it doesn't seem worth it
				off = int(ubxbin.MgaIniTimeUTCLeapSecsUnknown)
				tAccS += 1
			}
		}
		if off <= math.MaxInt8 {
			leapSecs = int8(off)
		}
	}
	if tAccS > math.MaxUint16 {
		return nil, fmt.Errorf("time accuracy %v is too large for UBX-MGA-INI-TIME-UTC", acc)
	}
	utc := ubxbin.MgaIniTimeUTC{
		Ref:      ubxbin.MgaIniTimeRefSourceNone,
		LeapSecs: leapSecs,
		Year:     uint16(t.Year()),
		Month:    uint8(t.Month()),
		Day:      uint8(t.Day()),
		Hour:     uint8(t.Hour()),
		Minute:   uint8(t.Minute()),
		Second:   uint8(t.Second()),
		Ns:       uint32(t.Nanosecond()),
		TAccS:    uint16(tAccS),
		TAccNs:   tAccNs,
	}
	utc.Bitfield0 |= ubxbin.MgaIniTimeTrustedSource
	return &ubxbin.MgaIni{
		MgaIniFixed: ubxbin.MgaIniFixed{
			Type:    ubxbin.MgaIniTypeTimeUTC,
			Version: ubxbin.MgaIniVersion,
		},
		Payload: &utc,
	}, nil
}

// mgaOSNMAMerkleTree creates an UBX-MGA-GAL-OSNMA_MERKLE message with the given Merkle tree root.
func mgaOSNMAMerkle(rootNode [32]byte, future bool) *ubxbin.MgaGal {
	if rootNode == [32]byte{} {
		return nil
	}
	var bitfield0 ubxbin.MgaGalOSNMAMerkleBitfield0
	if future {
		bitfield0 |= ubxbin.MgaGalOSNMAMerkleApplicabilityTimeFuture
	} else {
		bitfield0 |= ubxbin.MgaGalOSNMAMerkleApplicabilityTimeCurrent
	}
	merkle := ubxbin.MgaGalOSNMAMerkle{
		TreeNode:  rootNode,
		Bitfield0: bitfield0,
	}
	return &ubxbin.MgaGal{
		MgaGalFixed: ubxbin.MgaGalFixed{
			Type:    ubxbin.MgaGalTypeOSNMAMerkle,
			Version: ubxbin.MgaGalVersion,
		},
		Payload: &merkle,
	}
}
