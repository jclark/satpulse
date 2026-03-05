package novmsg

import (
	"fmt"
	"strconv"
	"testing"
)

var bestPosTests = []dataTestCase[UnicorePort]{
	{
		name:  "UM980 BESTPOS SINGLE",
		hex:   "aa44121c2a00000348008c4461a0660978d0920bb2f5eb091f001200000000001000000097a58c81b0762b404e359e3d43295940000020fcbbe71d40bda5f6c13d00000023c6c43fe995c23fb740184000000000000000000000a040341c1c00011211619759b49d",
		ascii: "#BESTPOSA,COM3,17548,97.0,FINE,2406,194171.000,166458802,31,18;SOL_COMPUTED,SINGLE,13.73181538431,100.64472904634,7.4763,-30.8309,WGS84,1.5373,1.5202,2.3789,\"\",0.000,5.000,52,28,28,0,1,12,11,61*d2f0bc98\r\n",
		hdr: MsgHdr[UnicorePort]{
			Port: UnicoreCOM3,
			CommonHdr: CommonHdr{
				Sequence:           17548,
				IdleTime:           Percentage(97),
				TimeStatus:         TimeStatusFine,
				Week:               2406,
				MillisecondsOfWeek: GPSec(194171000),
				RecvStatus:         166458802,
				Reserved:           31,
				Version:            18,
			},
		},
		value: &BestPos{Pos: Pos[SolStatus, PosType]{
			PSolStatus:    SolComputed,
			PosType:       PosSingle,
			Lat:           13.731815384310535,
			Lon:           100.64472904634496,
			Hgt:           7.476303042843938,
			Undulation:    -30.830926895141602,
			DatumID:       DatumWGS84,
			LatSigma:      1.5372966527938843,
			LonSigma:      1.520199894905090332,
			HgtSigma:      2.3789498805999756,
			StnID:         StationID{},
			DiffAge:       0.0,
			SolAge:        5.0,
			NumSVs:        52,
			NumSolnSVs:    28,
			NumSolnL1SVs:  28,
			NumSolnMulti:  0,
			Reserved:      1,
			ExtSolStat:    0x12,
			GalBDS3Sig:    0x11,
			GPSGLOBDS2Sig: 0x61,
		}},
		fixupValueForAscii: fixupBestPosForAscii,
		fixupHeaderForAscii: func(hdr MsgHdr[UnicorePort]) MsgHdr[UnicorePort] {
			hdr.IdleTime *= 2
			return hdr
		},
	},
}

var bestXYZTests = []dataTestCase[UnicorePort]{
	{
		name:  "UM980 BESTXYZ SINGLE",
		hex:   "aa44121cf100000370008c4461a0660958ff920b9d24ec09a922120000000000100000002384b1bc797731c1bcec76b0983b5741cc3d548fa9f336417004c73f1f1214405e0dcb3f0000000008000000a2ec27f5a7a1693f12d198802bdd64bff0644c1cfa45443f2fbc263c1fd7953c3470203c00000000000000000000000000000000351c1c00000211612dab705d",
		ascii: "#BESTXYZA,COM3,17548,97.0,FINE,2406,194183.000,166470813,20,18;SOL_COMPUTED,SINGLE,-1144697.7371,6090338.7573,1504169.5599,1.5548,2.3136,1.5863,SOL_COMPUTED,DOPPLER_VELOCITY,0.0031,-0.0025,0.0006,0.0102,0.0183,0.0098,\"\",0.000,0.000,0.000,53,28,28,0,0,02,11,61*2d35da95\r\n",
		hdr: MsgHdr[UnicorePort]{
			Port: UnicoreCOM3,
			CommonHdr: CommonHdr{
				Sequence:           17548,
				IdleTime:           Percentage(97),
				TimeStatus:         TimeStatusFine,
				Week:               2406,
				MillisecondsOfWeek: GPSec(194183000),
				RecvStatus:         166470813,
				Reserved:           8873,
				Version:            18,
			},
		},
		value: &BestXYZ{XYZ: XYZ[SolStatus, PosType]{
			PSolStatus:    SolComputed,
			PosType:       PosSingle,
			PX:            -1144697.7370836816,
			PY:            6090338.75725859,
			PZ:            1504169.5598791717,
			PXSigma:       1.5548229217529297,
			PYSigma:       2.313606023788452,
			PZSigma:       1.5863454341888428,
			VSolStatus:    SolComputed,
			VelType:       PosDopplerVelocity,
			VX:            0.0031288414404549584,
			VY:            -0.0025468682913701935,
			VZ:            0.0006186934702753482,
			VXSigma:       0.0101767024025321,
			VYSigma:       0.018291054293513298,
			VZSigma:       0.009792376309633255,
			StnID:         StationID{},
			VLatency:      0.0,
			DiffAge:       0.0,
			SolAge:        0.0,
			NumSVs:        53,
			NumSolnSVs:    28,
			NumSolnL1SVs:  28,
			NumSolnMulti:  0,
			Reserved:      0,
			ExtSolStat:    0x02,
			GalBDS3Sig:    0x11,
			GPSGLOBDS2Sig: 0x61,
		}},
		fixupValueForAscii: fixupBestXYZForAscii,
		fixupHeaderForAscii: func(hdr MsgHdr[UnicorePort]) MsgHdr[UnicorePort] {
			hdr.IdleTime *= 2
			hdr.Reserved = 20
			return hdr
		},
	},
}

var bestGNSSVelTests = []dataTestCase[Port]{
	{
		name:  "Bynav M20 BESTGNSSVEL DOPPLER_VELOCITY",
		hex:   "aa44121c960500602c000000c7b46609d0f55a1b00000000000010030000000008000000eee024b900000000222d2dc29278a03f6159e9a20e9663c04006c5cadfb33abf00000000b46d21fa",
		ascii: "#BESTGNSSVELA,COM3,0,99.7,FINESTEERING,2406,458946.000,00000000,0000,784;SOL_COMPUTED,DOPPLER_VELOCITY,-0.000,0.000,0.0322,-156.689287,-0.0004,0.0*caa00864\r\n",
		hdr: MsgHdr[Port]{
			Port: COM3,
			CommonHdr: CommonHdr{
				Sequence:           0,
				IdleTime:           Percentage(199),
				TimeStatus:         TimeStatusFineSteering,
				Week:               2406,
				MillisecondsOfWeek: GPSec(458946000),
				RecvStatus:         0,
				Reserved:           0,
				Version:            784,
			},
		},
		value: &BestGNSSVel{Vel: Vel[PosType]{
			SolStatus: SolComputed,
			VelType:   PosDopplerVelocity,
			Latency:   -0.00015724051627330482,
			Age:       0,
			HorSpd:    0.03216990108793484,
			TrkGnd:    -156.68928666664127,
			VertSpd:   -0.0004074498526912308,
			Reserved:  0,
		}},
		fixupValueForAscii: fixupBestGNSSVelForAscii,
	},
}

func TestBestGNSSVelBinary(t *testing.T) {
	testDataBin(t, bestGNSSVelTests, BinRegistry())
}

func TestBestGNSSVelAscii(t *testing.T) {
	testDataAscii(t, bestGNSSVelTests, AsciiRegistry())
}

func TestBestXYZBinary(t *testing.T) {
	testDataBin(t, bestXYZTests, BinRegistry())
}

func TestBestXYZAscii(t *testing.T) {
	testDataAscii(t, bestXYZTests, AsciiRegistry())
}

func TestBestPosBinary(t *testing.T) {
	testDataBin(t, bestPosTests, BinRegistry())
}

func TestBestPosAscii(t *testing.T) {
	testDataAscii(t, bestPosTests, AsciiRegistry())
}

var psrDopTests = []dataTestCase[Port]{
	{
		name:  "Bynav M2 PSRDOP",
		hex:   "aa44121cae000060cc000000c7b4670970addb0100000000000010038d397c3f1d47593fd6f5e33e40762b3fb517003f0000a0402c00000005000000060000000b0000000e000000110000001500000016000000180000001e000000220000002d0000002e00000031000000470000005000000052000000550000005c0000006100000062000000660000006b0000006c0000006e0000006f0000007000000071000000720000007300000074000000750000007600000077000000790000008100000082000000830000008b0000008f000000900000009100000094000000a4000000a500000002e06f0d",
		ascii: "#PSRDOPA,COM3,0,99.7,FINESTEERING,2407,31174.000,00000000,0000,784;0.9853,0.8487,0.4452,0.6698,0.5004,5.0,44,5,6,11,14,17,21,22,24,30,34,45,46,49,71,80,82,85,92,97,98,102,107,108,110,111,112,113,114,115,116,117,118,119,121,129,130,131,139,143,144,145,148,164,165*c36464a9\r\n",
		hdr: MsgHdr[Port]{
			Port: COM3,
			CommonHdr: CommonHdr{
				Sequence:           0,
				IdleTime:           Percentage(199),
				TimeStatus:         TimeStatusFineSteering,
				Week:               2407,
				MillisecondsOfWeek: GPSec(31174000),
				RecvStatus:         0,
				Reserved:           0,
				Version:            784,
			},
		},
		value: &PsrDop{
			PsrDopInitChunk: PsrDopInitChunk{
				GDOP:    0.9852531552314758,
				PDOP:    0.848741352558136,
				HDOP:    0.4452349543571472,
				HTDOP:   0.6697731018066406,
				TDOP:    0.5003617405891418,
				Cutoff:  5.0,
				NumPRNs: 44,
			},
			PRNs: []PsrDopPRN{
				{5}, {6}, {11}, {14}, {17}, {21}, {22}, {24},
				{30}, {34}, {45}, {46}, {49}, {71}, {80}, {82},
				{85}, {92}, {97}, {98}, {102}, {107}, {108}, {110},
				{111}, {112}, {113}, {114}, {115}, {116}, {117}, {118},
				{119}, {121}, {129}, {130}, {131}, {139}, {143}, {144},
				{145}, {148}, {164}, {165},
			},
		},
		fixupValueForAscii: fixupPsrDopForAscii,
	},
}

func TestPsrDopBinary(t *testing.T) {
	testDataBin(t, psrDopTests, BinRegistry())
}

func TestPsrDopAscii(t *testing.T) {
	testDataAscii(t, psrDopTests, AsciiRegistry())
}

func fixupPsrDopForAscii(msg MsgBody) MsgBody {
	m := msg.(*PsrDop)
	r := *m
	fixupFloat32(&r.GDOP, "%.4f")
	fixupFloat32(&r.PDOP, "%.4f")
	fixupFloat32(&r.HDOP, "%.4f")
	fixupFloat32(&r.HTDOP, "%.4f")
	fixupFloat32(&r.TDOP, "%.4f")
	fixupFloat32(&r.Cutoff, "%.1f")
	return &r
}

func fixupBestPosForAscii(msg MsgBody) MsgBody {
	m := msg.(*BestPos)
	r := *m
	fixupFloat(&r.Lat, "%.11f")
	fixupFloat(&r.Lon, "%.11f")
	fixupFloat(&r.Hgt, "%.4f")
	fixupFloat32(&r.Undulation, "%.4f")
	fixupFloat32(&r.LatSigma, "%.4f")
	fixupFloat32(&r.LonSigma, "%.4f")
	fixupFloat32(&r.HgtSigma, "%.4f")
	return &r
}

func fixupBestXYZForAscii(msg MsgBody) MsgBody {
	m := msg.(*BestXYZ)
	r := *m
	fixupFloat(&r.PX, "%.4f")
	fixupFloat(&r.PY, "%.4f")
	fixupFloat(&r.PZ, "%.4f")
	fixupFloat32(&r.PXSigma, "%.4f")
	fixupFloat32(&r.PYSigma, "%.4f")
	fixupFloat32(&r.PZSigma, "%.4f")
	fixupFloat(&r.VX, "%.4f")
	fixupFloat(&r.VY, "%.4f")
	fixupFloat(&r.VZ, "%.4f")
	fixupFloat32(&r.VXSigma, "%.4f")
	fixupFloat32(&r.VYSigma, "%.4f")
	fixupFloat32(&r.VZSigma, "%.4f")
	return &r
}

func fixupBestGNSSVelForAscii(msg MsgBody) MsgBody {
	m := msg.(*BestGNSSVel)
	r := *m
	fixupFloat32(&r.Latency, "%.3f")
	fixupFloat32(&r.Age, "%.3f")
	fixupFloat(&r.HorSpd, "%.4f")
	fixupFloat(&r.TrkGnd, "%.6f")
	fixupFloat(&r.VertSpd, "%.4f")
	fixupFloat32(&r.Reserved, "%.1f")
	return &r
}

// fixupFloat32 simulates the receiver's float formatting for float32 values
func fixupFloat32(val *float32, format string) {
	str := fmt.Sprintf(format, *val)
	result, err := strconv.ParseFloat(str, 32)
	if err != nil {
		panic(fmt.Sprintf("failed to round-trip float32 %v: %v", *val, err))
	}
	*val = float32(result)
}
