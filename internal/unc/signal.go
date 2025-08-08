package unc

import "github.com/jclark/satpulse/internal/gpsprot"

// signalGroups maps Unicore signal group index to gpsprot.SignalSet
// Based on Table 4-31 from Unicore NebulasIV Reference Manual R1.6
var signalGroups = []gpsprot.SignalSet{
	// 0: Disable slave antenna
	0,

	// 1: BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b), QZSS (L1C/A, L2C, L5)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5,
	),

	// 2: BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L1C, L2C, L2P(Y), L5), GLO (G1, G2, G3), GAL (E1, E5a, E5b, E6), QZSS (L1C/A, L1C, L2C, L5), NavIC (L5)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2, gpsprot.SigGLOL3,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b, gpsprot.SigGALE6,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL1C, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5,
		// NavIC
		gpsprot.SigNAVICL5,
	),

	// 3: BDS (B1I, B3I, B1C, B2b-PPP), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b, E6), QZSS (L1C/A, L2C, L5)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b, gpsprot.SigGALE6,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2b,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5,
	),

	// 4: BDS (B1I, B2I, B3I), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b), QZSS (L1C/A, L2C, L5)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5,
	),

	// 5: BDS (B1I, B2I, B3I), GPS (L1C/A, L2C/L2P), GLO (G1, G2), GAL (E1, E5b), QZSS (L1C/A, L2C)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5b,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C,
	),

	// 6: BDS (B1I, B3I), GPS (L1C/A, L2C/L2P), GLO (G1, G2), GAL (E1, E5b), QZSS (L1C/A, L2C)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5b,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB3I,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C,
	),

	// 7: BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b), QZSS (L1C/A, L2C, L5)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5,
	),

	// 8: GPS (L1C/A, L2C/L2P, L5), BDS (B1I, B3I, B1C, B2a), GAL (E1, E5a, E5b)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2a,
	),

	// 9: BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L2P(Y)/L2C, L5), GLO (L1C/A, L2C/A), GAL (E1C, E5A, E5B), QZSS (L1C/A, L2C, L5)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2P, gpsprot.SigGPSL2C, gpsprot.SigGPSL5,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5,
	),

	// 10: BDS (B1I, B2I, B3I, B1C, B2a, B2b), GPS (L1C/A, L2C/L2P, L5), GLO (G1, G2), GAL (E1, E5a, E5b), QZSS (L1C/A, L2C, L5, L6)
	gpsprot.SignalSetOf(
		// GPS
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
		// GLONASS
		gpsprot.SigGLOL1, gpsprot.SigGLOL2,
		// Galileo
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b,
		// BeiDou
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
		// QZSS
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5, gpsprot.SigQZSSL6,
	),
}

// maskSignalEntry represents a MASK command argument and its corresponding SignalSet
type maskSignalEntry struct {
	name      string
	signalSet gpsprot.SignalSet
}

// maskSignalList ordered array of MASK command arguments to gpsprot.SignalSet
// Based on Table 5-5 from Unicore protocol documentation
// Ordered so that supersets come before subsets for optimal mask generation
var maskSignalList = []maskSignalEntry{
	// System-level masks (union of all supported frequencies)
	{"GPS", gpsprot.SignalSetOf(
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, gpsprot.SigGPSL5,
	)},
	{"BDS", gpsprot.SignalSetOf(
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigBDSB3I, gpsprot.SigBDSB1C, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b,
	)},
	{"GLO", gpsprot.SignalSetOf(
		gpsprot.SigGLOL1, gpsprot.SigGLOL2, gpsprot.SigGLOL3,
	)},
	{"GAL", gpsprot.SignalSetOf(
		gpsprot.SigGALE1, gpsprot.SigGALE5a, gpsprot.SigGALE5b, gpsprot.SigGALE6,
	)},
	{"QZSS", gpsprot.SignalSetOf(
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL1C, gpsprot.SigQZSSL2C, gpsprot.SigQZSSL5, gpsprot.SigQZSSL6,
	)},
	{"IRNSS", gpsprot.SignalSetOf(gpsprot.SigNAVICL5)},

	// GPS frequency-specific masks
	{"L1", gpsprot.SignalSetOf(
		gpsprot.SigGPSL1CA, gpsprot.SigGPSL1C, // When masking L1, it disables L1C/A and L1C
	)},
	{"L1CA", gpsprot.SignalSetOf(gpsprot.SigGPSL1CA)},
	{"L1C", gpsprot.SignalSetOf(gpsprot.SigGPSL1C)},
	{"L2", gpsprot.SignalSetOf(
		gpsprot.SigGPSL2C, gpsprot.SigGPSL2P, // When masking L2, it disables L2C and L2P
	)},
	{"L2C", gpsprot.SignalSetOf(gpsprot.SigGPSL2C)},
	{"L2P", gpsprot.SignalSetOf(gpsprot.SigGPSL2P)},
	{"L5", gpsprot.SignalSetOf(gpsprot.SigGPSL5)},

	// BDS frequency-specific masks
	{"B1", gpsprot.SignalSetOf(
		gpsprot.SigBDSB1I, gpsprot.SigBDSB1C, // When masking B1, it disables B1I and BD3B1C
	)},
	{"B2", gpsprot.SignalSetOf(
		gpsprot.SigBDSB2I, gpsprot.SigBDSB2a, gpsprot.SigBDSB2b, // When masking B2, it disables B2I, B2a and B2b
	)},
	{"B3", gpsprot.SignalSetOf(gpsprot.SigBDSB3I)}, // When masking B3, it disables B3I
	{"B1I", gpsprot.SignalSetOf(gpsprot.SigBDSB1I)},
	{"B2I", gpsprot.SignalSetOf(gpsprot.SigBDSB2I)},
	{"B3I", gpsprot.SignalSetOf(gpsprot.SigBDSB3I)},
	{"BD3B1C", gpsprot.SignalSetOf(gpsprot.SigBDSB1C)},
	{"BD3B2A", gpsprot.SignalSetOf(gpsprot.SigBDSB2a)},
	{"BD3B2B", gpsprot.SignalSetOf(gpsprot.SigBDSB2b)},

	// GLONASS frequency-specific masks
	{"R1", gpsprot.SignalSetOf(gpsprot.SigGLOL1)},
	{"R2", gpsprot.SignalSetOf(gpsprot.SigGLOL2)},
	{"R3", gpsprot.SignalSetOf(gpsprot.SigGLOL3)},

	// Galileo frequency-specific masks
	{"E1", gpsprot.SignalSetOf(gpsprot.SigGALE1)},
	{"E5a", gpsprot.SignalSetOf(gpsprot.SigGALE5a)},
	{"E5b", gpsprot.SignalSetOf(gpsprot.SigGALE5b)},
	{"E6C", gpsprot.SignalSetOf(gpsprot.SigGALE6)},

	// QZSS frequency-specific masks
	{"Q1", gpsprot.SignalSetOf(
		gpsprot.SigQZSSL1CA, gpsprot.SigQZSSL1C, // When masking Q1, it disables QZSS L1C/A and QZSS L1C
	)},
	{"Q2", gpsprot.SignalSetOf(gpsprot.SigQZSSL2C)}, // When masking Q2, it disables QZSS L2C
	{"Q5", gpsprot.SignalSetOf(gpsprot.SigQZSSL5)},  // When masking Q5, it disables QZSS L5
	{"Q1CA", gpsprot.SignalSetOf(gpsprot.SigQZSSL1CA)}, // QZSS L1C/A
	{"Q1C", gpsprot.SignalSetOf(gpsprot.SigQZSSL1C)},  // QZSS L1C
	{"Q2C", gpsprot.SignalSetOf(gpsprot.SigQZSSL2C)},  // QZSS L2C

	// IRNSS frequency-specific masks
	{"I5", gpsprot.SignalSetOf(gpsprot.SigNAVICL5)}, // When masking I5, it disables IRNSS L5
}

// maskSignalMap provides fast lookup of mask names to signal sets
var maskSignalMap map[string]gpsprot.SignalSet

func init() {
	// Create map from list for fast lookups
	maskSignalMap = make(map[string]gpsprot.SignalSet, len(maskSignalList))
	for _, entry := range maskSignalList {
		maskSignalMap[entry.name] = entry.signalSet
	}
}