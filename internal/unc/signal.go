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