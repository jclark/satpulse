package rtcmbin

import (
	"errors"
	"fmt"
	"math/bits"
)

// MSM7Convert converts an MSM7 message (MSMHiRes) to a standard-resolution
// MSM at the given level. Currently only level 4 is supported.
// Returns an error if the input is not MSM7, the target level is unsupported,
// the input is structurally inconsistent, or it contains reserved DF407 values.
func MSM7Convert(m7 *MSMHiRes, level int) (*MSM, error) {
	if m7.MSMLevel() != 7 {
		return nil, fmt.Errorf("only conversion from MSM7 is supported, got MSM%d", m7.MSMLevel())
	}
	if level != 4 {
		return nil, fmt.Errorf("only conversion to MSM4 is supported, got level %d", level)
	}
	nsat := m7.Nsat()
	if len(m7.Sat.RangeInt) != nsat || len(m7.Sat.RangeMod) != nsat ||
		len(m7.Sat.ExtInfo) != nsat || len(m7.Sat.PhaseRate) != nsat {
		return nil, errors.New("satellite data slice lengths do not match satellite mask")
	}
	cmBits := m7.cellMaskBits()
	if cmBits > 64 || m7.CellMask>>cmBits != 0 {
		return nil, errors.New("cell mask has bits set beyond satellite * signal count")
	}
	ncell := bits.OnesCount64(m7.CellMask)
	if len(m7.Sig.Pseudorange) != ncell || len(m7.Sig.PhaseRange) != ncell ||
		len(m7.Sig.LockTime) != ncell || len(m7.Sig.HalfCycle) != ncell ||
		len(m7.Sig.CNR) != ncell || len(m7.Sig.PhaseRate) != ncell {
		return nil, errors.New("signal data slice lengths do not match cell mask")
	}
	m4 := &MSM{
		MSMHeader: m7.MSMHeader,
		CellMask:  m7.CellMask,
	}
	m4.MsgNum -= 3
	m4.Sat.RangeInt = m7.Sat.RangeInt
	m4.Sat.RangeMod = m7.Sat.RangeMod
	m4.Sat.ExtInfo = Uint8Slice{}
	m4.Sat.PhaseRate = []int16{}
	m4.Sig.Pseudorange = make([]int16, ncell)
	m4.Sig.PhaseRange = make([]int32, ncell)
	m4.Sig.LockTime = make(Uint8Slice, ncell)
	m4.Sig.HalfCycle = m7.Sig.HalfCycle
	m4.Sig.CNR = make(Uint8Slice, ncell)
	m4.Sig.PhaseRate = []int16{}
	for i := range ncell {
		// Fine pseudorange: DF405 (int20) -> DF400 (int15)
		if m7.Sig.Pseudorange[i] == -524288 {
			m4.Sig.Pseudorange[i] = -16384
		} else {
			v := roundShift(m7.Sig.Pseudorange[i], 5)
			m4.Sig.Pseudorange[i] = int16(max(min(v, 16383), -16383))
		}
		// Fine phase range: DF406 (int24) -> DF401 (int22)
		if m7.Sig.PhaseRange[i] == -8388608 {
			m4.Sig.PhaseRange[i] = -2097152
		} else {
			v := roundShift(m7.Sig.PhaseRange[i], 2)
			m4.Sig.PhaseRange[i] = max(min(v, 2097151), -2097151)
		}
		// CNR: DF408 (uint10) -> DF403 (uint6)
		cnr := m7.Sig.CNR[i]
		if cnr == 0 {
			m4.Sig.CNR[i] = 0
		} else {
			v := (cnr + 8) / 16
			m4.Sig.CNR[i] = uint8(max(min(v, 63), 1))
		}
		// Lock time: DF407 (uint10) -> DF402 (uint4)
		ms, ok := lockTimeMsFromDF407(m7.Sig.LockTime[i])
		if !ok {
			return nil, fmt.Errorf("reserved DF407 lock time indicator %d", m7.Sig.LockTime[i])
		}
		m4.Sig.LockTime[i] = lockTimeToDF402(ms)
	}
	return m4, nil
}

// roundShift rounds v by shifting right n bits, rounding half away from zero.
func roundShift(v int32, n uint) int32 {
	half := int32(1) << (n - 1)
	if v >= 0 {
		return (v + half) >> n
	}
	return (v + half - 1) >> n
}

// lockTimeMsFromDF407 returns the minimum lock time in ms for a DF407
// indicator value. Returns (ms, true) for valid indicators 0-704, or
// (0, false) for reserved values 705-1023.
func lockTimeMsFromDF407(ind uint16) (uint32, bool) {
	if ind > 704 {
		return 0, false
	}
	if ind <= 63 {
		return uint32(ind), true
	}
	// Each range of 32 indicators doubles the coefficient k.
	// ms = k * offset_within_range + start_ms_for_range
	r := (ind - 64) / 32
	k := uint32(2) << r
	ind0 := 64 + 32*r
	return k*uint32(ind-ind0) + 64*(1<<r), true
}

// lockTimeToDF402 returns the DF402 indicator for a lock time in ms.
func lockTimeToDF402(ms uint32) uint8 {
	// DF402 thresholds: 0, 32, 64, 128, 256, 512, ..., 524288
	// Each threshold doubles starting from 32.
	if ms < 32 {
		return 0
	}
	// Find largest indicator whose threshold <= ms.
	// Threshold for indicator i (1-15) is 32 * 2^(i-1) = 16 << i.
	var ind uint8 = 1
	for ind < 15 && 16<<(ind+1) <= ms {
		ind++
	}
	return ind
}
