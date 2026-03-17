package sdbpbin

import (
	"testing"
)

// Captured packets from Taidou T303-5D (2026-03-17).
var datPackets = []struct {
	name string
	hex  string
	want Msg
}{
	{
		"DAT-UTCT2",
		"233e061f1f0048002d0d0b02ea0703110d173741a002000400000012ffffffff000000000028d8",
		&DatUTCT2{
			DatNavHeader: DatNavHeader{221053000},
			Valid:        DatUTCT2HMS | DatUTCT2YMD | DatUTCT2LeapCorr, // 11 = 0b1011
			RefGrid:      DatUTCT2RefGPS,
			Year:         2026, Month: 3, Day: 17,
			Hour: 13, Min: 23, Sec: 55,
			SecFrac: 172097, Accuracy: 4,
			LeapSec: 18, LeapCountdown: -1,
			LeapChange: 0, LeapYear: 0, LeapMonth: 0, LeapDay: 0,
		},
	},
	{
		"DAT-GPSU",
		"233e062d230000000000000030be000000000000e0bc0000000000000000003006006a09128909071246da",
		&DatGPSU{DatGNSSU{
			A0: -3.725290298461914e-9, A1: -1.7763568394002505e-15, A2: 0,
			TOT: 405504, WNOT: 2410,
			DeltaTLS: 18, WNLSF: 2441, DN: 7, DeltaTLSF: 18,
		}},
	},
	{
		"DAT-BDSU",
		"233e062c2300000000000000d8bd00000000000007bd0000000000000000b04e03001e04043d0006041cb5",
		&DatBDSU{DatGNSSU{
			A0: -8.731149137020111e-11, A1: -1.021405182655144e-14, A2: 0,
			TOT: 216752, WNOT: 1054,
			DeltaTLS: 4, WNLSF: 61, DN: 6, DeltaTLSF: 4,
		}},
	},
	{
		"DAT-LLA3",
		"233e061d4000d8e1420d00133529ea0703110d2f68bfcebf898743295940b2e59b83b0762b40fc9b3941185ae4c1630300001c05000057c3b23b0200000000000000ffffffffe057",
		&DatLLA3{
			DatNavHeader: DatNavHeader{222487000},
			CoordSys: 0, Valid: 19, TrackedSats: 53, FixSats: 41,
			Year: 2026, Month: 3, Day: 17, Hour: 13, Min: 47, SecMs: 49000,
			Lon: 100.6447466702659, Lat: 13.731815445690561,
			AltMSL: 11.600582, GeoidSep: -28.543991,
			HAcc: 867, VAcc: 1308,
			GroundSpeed: 0.005455415, SpeedAcc: 2,
			Heading: 0, HeadingAcc: 4294967295,
		},
	},
	{
		"DAT-ECEF2",
		"233e061b4b00d8e1420d133529ea0703110d2f68bfd0b1d1b97c7731c1a46e65239a3b57417818cf0eabf33641d9020000090500000902000034f6e0bbc1a3f83b2e3c553b0200000003000000010000001663",
		&DatECEF2{
			DatNavHeader: DatNavHeader{222487000},
			Valid: 19, TrackedSats: 53, FixSats: 41,
			Year: 2026, Month: 3, Day: 17, Hour: 13, Min: 47, SecMs: 49000,
			X: -1144700.7258559354, Y: 6090344.55306593, Z: 1504171.0578475278,
			XAcc: 729, YAcc: 1289, ZAcc: 521,
			VX: -0.006865287, VY: 0.0075878804, VZ: 0.003253709,
			VXAcc: 2, VYAcc: 3, VZAcc: 1,
		},
	},
	{
		"DAT-NED3",
		"233e061e2800d8e1420d00133529ea0703110d2f68bffccc8e3a7929af3ba98617bc010000000200000003000000513c",
		&DatNED3{
			DatNavHeader: DatNavHeader{222487000},
			CoordSys: 0, Valid: 19, TrackedSats: 53, FixSats: 41,
			Year: 2026, Month: 3, Day: 17, Hour: 13, Min: 47, SecMs: 49000,
			VN: 0.001089483, VE: 0.00534552, VD: -0.009248414,
			VNAcc: 1, VEAcc: 2, VDAcc: 3,
		},
	},
	{
		"DAT-DOP",
		"233e06131000d8e1420d0329640058002c004c003100c21e",
		&DatDOP{
			DatNavHeader: DatNavHeader{222487000},
			Valid: 3, FixSats: 41,
			GDOP: 100, PDOP: 88, HDOP: 44, VDOP: 76, TDOP: 49,
		},
	},
}

func TestDatParseAndRoundTrip(t *testing.T) {
	for _, tc := range datPackets {
		t.Run(tc.name, func(t *testing.T) {
			got := parseAndCompare(t, tc.hex, tc.want)
			serializeRoundTrip(t, got, tc.hex)
		})
	}
}
