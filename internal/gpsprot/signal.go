package gpsprot

import (
	"math/bits"
	"strings"
)

type Signal int

// SignalSet is a bitmask representing a set of GNSS signals.
// SignalSet makes it convenient to manipulate sets of signals based on GNSS and band.
// The Signal type gives the index of the signal in the set.
// The idea is that the 64 bits are divided into 6 groups of 11 bits each,
// where there is a group for each GNSS constellation, and the 11 bits is a Band bitmask.
// The 2-bit overflow does not matter since the last constellation is NAVIC,
// which does not use all bits in the Band bitmask.
type SignalSet uint64

// Offsets for each constellation
const (
	sigBaseGPS   = (Signal(GPS) - 1) * sigIndexCount
	sigBaseGAL   = (Signal(GAL) - 1) * sigIndexCount
	sigBaseBDS   = (Signal(BDS) - 1) * sigIndexCount
	sigBaseGLO   = (Signal(GLO) - 1) * sigIndexCount
	sigBaseQZSS  = (Signal(QZSS) - 1) * sigIndexCount
	sigBaseNAVIC = (Signal(NAVIC) - 1) * sigIndexCount
)

// These are added to the base to get the signal number.
// They give the index in the Band bitmask for each GNSS.
const (
	sigIndexL1Legacy Signal = iota
	sigIndexL1Modern
	sigIndexL1Augment
	sigIndexL2Legacy
	sigIndexL2Modern
	sigIndexL5
	sigIndexL5Augment
	sigIndexE5b
	sigIndexE5bAugment
	sigIndexE6
	sigIndexE6Augment
	sigIndexCount
)

const (
	// GPS signals
	SigGPSL1CA Signal = sigBaseGPS + sigIndexL1Legacy // L1 C/A (1575.42 MHz)
	SigGPSL1C  Signal = sigBaseGPS + sigIndexL1Modern // L1C (1575.42 MHz)
	SigGPSL2P  Signal = sigBaseGPS + sigIndexL2Legacy // L2P(Y) (1227.60 MHz)
	SigGPSL2C  Signal = sigBaseGPS + sigIndexL2Modern // L2C (1227.60 MHz)
	SigGPSL5   Signal = sigBaseGPS + sigIndexL5       // L5 (1176.45 MHz)

	// GLONASS signals
	SigGLOL1   Signal = sigBaseGLO + sigIndexL1Legacy // G1 (1598–1609 MHz)
	SigGLOL1OC Signal = sigBaseGLO + sigIndexL1Modern // L1OC (1600.995 MHz)
	SigGLOL2   Signal = sigBaseGLO + sigIndexL2Legacy // G2 (1243–1252 MHz)
	SigGLOL2OC Signal = sigBaseGLO + sigIndexL2Modern // L2OC (1248.06 MHz)
	SigGLOL3   Signal = sigBaseGLO + sigIndexE5b      // G3 (1202.025 MHz)

	// Galileo signals
	SigGALE1  Signal = sigBaseGAL + sigIndexL1Modern  // E1 (1575.42 MHz)
	SigGALE5a Signal = sigBaseGAL + sigIndexL5        // E5a (1176.45 MHz)
	SigGALE5b Signal = sigBaseGAL + sigIndexE5b       // E5b (1207.14 MHz)
	SigGALE6  Signal = sigBaseGAL + sigIndexE6Augment // E6 (1278.75 MHz)

	// BeiDou signals
	SigBDSB1I Signal = sigBaseBDS + sigIndexL1Legacy   // B1I (1561.098 MHz)
	SigBDSB1C Signal = sigBaseBDS + sigIndexL1Modern   // B1C (1575.42 MHz)
	SigBDSB2I Signal = sigBaseBDS + sigIndexE5b        // B2I (1207.14 MHz)
	SigBDSB2b Signal = sigBaseBDS + sigIndexE5bAugment // B2b (1207.14 MHz)
	SigBDSB2a Signal = sigBaseBDS + sigIndexL5         // B2a (1176.45 MHz)
	SigBDSB3I Signal = sigBaseBDS + sigIndexE6         // B3I (1268.52 MHz)

	// QZSS signals
	SigQZSSL1CA Signal = sigBaseQZSS + sigIndexL1Legacy  // L1 C/A (1575.42 MHz)
	SigQZSSL1C  Signal = sigBaseQZSS + sigIndexL1Modern  // L1C (1575.42 MHz)
	SigQZSSL1S  Signal = sigBaseQZSS + sigIndexL1Augment // L1S (1575.42 MHz)
	SigQZSSL2C  Signal = sigBaseQZSS + sigIndexL2Modern  // L2C (1227.60 MHz)
	SigQZSSL5   Signal = sigBaseQZSS + sigIndexL5        // L5 (1176.45 MHz)
	SigQZSSL5S  Signal = sigBaseQZSS + sigIndexL5Augment // L5 (1176.45 MHz)
	SigQZSSL6   Signal = sigBaseQZSS + sigIndexE6Augment // L6 (1278.75 MHz)

	// NavIC signals
	SigNAVICL1 Signal = sigBaseNAVIC + sigIndexL1Modern // L1 (1575.42 MHz)
	SigNAVICL5 Signal = sigBaseNAVIC + sigIndexL5       // L5 (1176.45 MHz)

	// SBAS signals
	SigSBASL1CA Signal = sigBaseGPS + sigIndexL1Augment // L1 C/A (1575.42 MHz)
	SigSBASL5   Signal = sigBaseGPS + sigIndexL5Augment // L5 (1176.45 MHz)
)

// SignalSetOf creates a SignalSet from a list of signals.
func SignalSetOf(sig ...Signal) SignalSet {
	var ss SignalSet
	for _, s := range sig {
		ss |= 1 << s
	}
	return ss
}

// Signal sets for each GNSS constellation
const (
	SigSetGPS     SignalSet = (1 << SigGPSL1CA) | (1 << SigGPSL1C) | (1 << SigGPSL2P) | (1 << SigGPSL2C) | (1 << SigGPSL5)
	SigSetGLO     SignalSet = (1 << SigGLOL1) | (1 << SigGLOL1OC) | (1 << SigGLOL2) | (1 << SigGLOL2OC) | (1 << SigGLOL3)
	SigSetGAL     SignalSet = (1 << SigGALE1) | (1 << SigGALE5a) | (1 << SigGALE5b) | (1 << SigGALE6)
	SigSetBDS     SignalSet = (1 << SigBDSB1I) | (1 << SigBDSB1C) | (1 << SigBDSB2I) | (1 << SigBDSB2b) | (1 << SigBDSB2a) | (1 << SigBDSB3I)
	SigSetQZSS    SignalSet = (1 << SigQZSSL1CA) | (1 << SigQZSSL1C) | (1 << SigQZSSL1S) | (1 << SigQZSSL2C) | (1 << SigQZSSL5) | (1 << SigQZSSL5S) | (1 << SigQZSSL6)
	SigSetNAVIC   SignalSet = (1 << SigNAVICL1) | (1 << SigNAVICL5)
	SigSetSBAS    SignalSet = (1 << SigSBASL1CA) | (1 << SigSBASL5)
)

const (
	SigSetMajor   SignalSet = SigSetGPS | SigSetGLO | SigSetGAL | SigSetBDS // signals for major GNSS constellations
	SigSetAugment SignalSet = SigSetSBAS | (1 << SigGALE6) | (1 << SigBDSB2b) | (1 << SigQZSSL1S) | (1 << SigQZSSL5S) | (1 << SigQZSSL6)
	SigSetAll     SignalSet = SigSetGPS | SigSetGLO | SigSetGAL | SigSetBDS | SigSetQZSS | SigSetNAVIC | SigSetSBAS
)

type Band uint16

// E5b and L5 could be considered the same band: you can | them together to represent this
// I prefer to keep L5 for just 1176.45 MHz, and then use E5b (for lack of a better name) for the rest
const (
	BandL1      Band = Band((1 << sigIndexL1Legacy) | (1 << sigIndexL1Modern) | (1 << sigIndexL1Augment)) // 1575.42 - 1609 MHz, 1559-1610
	BandL2      Band = Band((1 << sigIndexL2Legacy) | (1 << sigIndexL2Modern))                            // 1227.60 - 1252 MHz, 1215-1252
	BandL5      Band = Band((1 << sigIndexL5) | (1 << sigIndexL5Augment))                                 // 1176.45 MHz, 1164-1210
	BandE5b     Band = Band((1 << sigIndexE5b) | (1 << sigIndexE5bAugment))                               // 1202.025 - 1207.14 MHz, 1164-1210
	BandE6      Band = Band((1 << sigIndexE6) | (1 << sigIndexE6Augment))                                 // 1268.52 - 1278.75 MHz, 1260-1300
	BandAll     Band = Band((1 << sigIndexCount) - 1)
	BandAugment Band = Band((1 << sigIndexL1Augment) | (1 << sigIndexL5Augment) | (1 << sigIndexE5bAugment) | (1 << sigIndexE6Augment)) // 1559-1610, 1164-1210, 1260-1300
)

// SignalSet returns the set of signals for a given GNSS in the Band.
func (b Band) SignalSet(gs ...GNSS) SignalSet {
	ss := SignalSet(0)
	for _, g := range gs {
		c := g
		// The signal set numbering treats SBAS as being in the GPS constellation.
		if g == SBAS {
			c = GPS
		}
		s := (SignalSet(b) << ((Signal(c) - 1) * sigIndexCount)) & SigSetAll
		if g == GPS {
			s &^= SigSetSBAS
		} else if g == SBAS {
			s &= SigSetSBAS
		}
		ss |= s
	}
	return ss
}

func (sig Signal) GNSS() GNSS {
	if sig == SigSBASL1CA || sig == SigSBASL5 {
		return SBAS
	}
	return GNSS(sig/sigIndexCount + 1)
}

// Signals returns an iterator that yields each signal in the SignalSet.
func (ss SignalSet) Signals() func(yield func(Signal) bool) {
	return func(yield func(Signal) bool) {
		// Create a copy of the signal set to manipulate
		ss := ss

		// Continue until all bits are processed
		for ss != 0 {
			// Get the lowest set bit
			sig := Signal(bits.TrailingZeros64(uint64(ss)))
			// Yield this signal and check if we should continue
			if !yield(Signal(sig)) {
				return
			}
			// Clear the lowest bit and continue
			ss &= ss - 1 // Kernighan's bit clearing trick
		}
	}
}

func (ss SignalSet) Contains(sig Signal) bool {
	return ss&SignalSet(1<<sig) != 0
}

// String returns a human-readable representation of the SignalSet in the form "GPS[L1,L5],GAL[E1,E5b]"
// This leaves " C/A" off the end of the signal name to keep things short.
func (ss SignalSet) String() string {
	if ss == 0 {
		return "None"
	}

	// Group signals by GNSS (index 0 is unused, since GNSS values start at 1)
	gnssSignals := make([][]Signal, GNSSLast+1)
	for sig := range ss.Signals() {
		g := sig.GNSS()
		gnssSignals[g] = append(gnssSignals[g], sig)
	}

	// Build the output string
	var result strings.Builder
	first := true
	for g := GNSS(1); g <= GNSSLast; g++ {
		signals := gnssSignals[g]
		if len(signals) == 0 {
			continue
		}
		if !first {
			result.WriteString(",")
		}
		first = false
		result.WriteString(g.String())
		result.WriteString("[")
		sigFirst := true
		for _, sig := range signals {
			if !sigFirst {
				result.WriteString(",")
			}
			sigFirst = false
			// Remove the " C/A" suffix
			name, _, _ := strings.Cut(sigName[sig], " ")
			result.WriteString(name)
		}
		result.WriteString("]")
	}
	return result.String()
}

// sigName provides human-readable names for each signal
var sigName [64]string

func init() {
	// GPS signals
	sigName[SigGPSL1CA] = "L1 C/A"
	sigName[SigGPSL1C] = "L1C"
	sigName[SigGPSL2P] = "L2P"
	sigName[SigGPSL2C] = "L2C"
	sigName[SigGPSL5] = "L5"

	// GLONASS signals
	sigName[SigGLOL1] = "L1"
	sigName[SigGLOL1OC] = "L1OC"
	sigName[SigGLOL2] = "L2"
	sigName[SigGLOL2OC] = "L2OC"
	sigName[SigGLOL3] = "L3"

	// Galileo signals
	sigName[SigGALE1] = "E1"
	sigName[SigGALE5a] = "E5a"
	sigName[SigGALE5b] = "E5b"
	sigName[SigGALE6] = "E6"

	// BeiDou signals
	sigName[SigBDSB1I] = "B1I"
	sigName[SigBDSB1C] = "B1C"
	sigName[SigBDSB2I] = "B2I"
	sigName[SigBDSB2b] = "B2b"
	sigName[SigBDSB2a] = "B2a"
	sigName[SigBDSB3I] = "B3I"

	// QZSS signals
	sigName[SigQZSSL1CA] = "L1 C/A"
	sigName[SigQZSSL1C] = "L1C"
	sigName[SigQZSSL1S] = "L1S"
	sigName[SigQZSSL2C] = "L2C"
	sigName[SigQZSSL5] = "L5"
	sigName[SigQZSSL5S] = "L5S"
	sigName[SigQZSSL6] = "L6"

	// NavIC signals
	sigName[SigNAVICL1] = "L1"
	sigName[SigNAVICL5] = "L5"

	// SBAS signals
	sigName[SigSBASL1CA] = "L1 C/A"
	sigName[SigSBASL5] = "L5"
}
