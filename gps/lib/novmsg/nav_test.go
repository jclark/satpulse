package novmsg

import (
	"fmt"
	"maps"
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
		value: &BestPos{
			Pos: Pos[SolStatus, PosType]{
				PSolStatus:   SolComputed,
				PosType:      PosSingle,
				Lat:          13.731815384310535,
				Lon:          100.64472904634496,
				Hgt:          7.476303042843938,
				Undulation:   -30.830926895141602,
				DatumID:      DatumWGS84,
				LatSigma:     1.5372966527938843,
				LonSigma:     1.520199894905090332,
				HgtSigma:     2.3789498805999756,
				StnID:        StationID{},
				DiffAge:      0.0,
				SolAge:       5.0,
				NumSVs:       52,
				NumSolnSVs:   28,
				NumSolnL1SVs: 28,
				NumSolnMulti: 0,
			},
			PosFlags: PosFlags{
				Reserved:      1,
				ExtSolStat:    0x12,
				GalBDS3Sig:    0x11,
				GPSGLOBDS2Sig: 0x61,
			},
		},
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
		value: &BestXYZ{
			XYZ: XYZ[SolStatus, PosType]{
				PSolStatus:   SolComputed,
				PosType:      PosSingle,
				PX:           -1144697.7370836816,
				PY:           6090338.75725859,
				PZ:           1504169.5598791717,
				PXSigma:      1.5548229217529297,
				PYSigma:      2.313606023788452,
				PZSigma:      1.5863454341888428,
				VSolStatus:   SolComputed,
				VelType:      PosDopplerVelocity,
				VX:           0.0031288414404549584,
				VY:           -0.0025468682913701935,
				VZ:           0.0006186934702753482,
				VXSigma:      0.0101767024025321,
				VYSigma:      0.018291054293513298,
				VZSigma:      0.009792376309633255,
				StnID:        StationID{},
				VLatency:     0.0,
				DiffAge:      0.0,
				SolAge:       0.0,
				NumSVs:       53,
				NumSolnSVs:   28,
				NumSolnL1SVs: 28,
				NumSolnMulti: 0,
			},
			PosFlags: PosFlags{
				Reserved:      0,
				ExtSolStat:    0x02,
				GalBDS3Sig:    0x11,
				GPSGLOBDS2Sig: 0x61,
			},
		},
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

// sinoNavTests are binary and ASCII pairs from a SinoGNSS K901, each
// requested with LOG ONCE back to back so that both report the same
// solution. The ASCII header has fixed values in place of the binary
// header's idle time, receiver status, reserved and version fields, and the
// ASCII solution age can be one second more than the binary one.
var sinoNavTests = []dataTestCase[Port]{
	{
		name:  "SinoGNSS K901 BESTPOS SINGLE",
		hex:   "aa44121c2a00022048000000b7b48509a07a50210000100053ff020000000000100000005226d3bfb2762b40ff65ee4f432959400000a01ab1421e406fcef6c13d00000073b55e3e39167f3ecbb26b3f0000000000000000000040401e1d1d1dbf000019b9039eee",
		ascii: "#BESTPOSA,COM1,0,60.0,FINESTEERING,2437,558922.400,00000000,0000,1114;SOL_COMPUTED,SINGLE,13.73183249905,100.64473341256,7.5651,-30.8508,WGS84,0.2175,0.2491,0.9207,\"\",0.000,4.000,30,29,29,29,191,0,0,25*2c24324c\r\n",
		hdr:   sinoHdr(183, 558922400, 65363, 2),
		value: &SinoBestPos{
			Pos: Pos[SolStatus, SinoPosType]{
				PSolStatus:   SolComputed,
				PosType:      PosSingle,
				Lat:          13.731832499051198,
				Lon:          100.64473341256233,
				Hgt:          7.565128723159432,
				Undulation:   -30.850798,
				DatumID:      DatumWGS84,
				LatSigma:     0.21748905,
				LonSigma:     0.24910821,
				HgtSigma:     0.9206969,
				StnID:        StationID{},
				DiffAge:      0,
				SolAge:       3,
				NumSVs:       30,
				NumSolnSVs:   29,
				NumSolnL1SVs: 29,
				NumSolnMulti: 29,
			},
			SinoPosFlags: SinoPosFlags{Reserved: 191, SigMask: 25},
		},
		fixupValueForAscii: func(msg MsgBody) MsgBody {
			r := *msg.(*SinoBestPos)
			fixupPosForAscii(&r.Pos)
			r.SolAge = 4
			return &r
		},
		fixupHeaderForAscii: sinoHdrForAscii,
	},
	{
		name:  "SinoGNSS K901 PSRPOS SINGLE",
		hex:   "aa44121c2f00022048000000b8b485093095502100001000eeff01000000000010000000d3041cbab2762b408878305143295940000010f723e01d406fcef6c13d00000082485d3efd1b803e348aec3f2020202000000000000000001d1d1d1d0000001995c9209a",
		ascii: "#PSRPOSA,COM1,0,60.0,FINESTEERING,2437,558929.200,00000000,0000,1114;SOL_COMPUTED,SINGLE,13.73183232872,100.64473371252,7.4689,-30.8508,WGS84,0.2161,0.2502,1.8480,\"    \",0.000,0.000,29,29,29,29,0,0,0,25*085aec4c\r\n",
		hdr:   sinoHdr(184, 558929200, 65518, 1),
		value: &SinoPsrPos{
			Pos: Pos[SolStatus, SinoPosType]{
				PSolStatus:   SolComputed,
				PosType:      PosSingle,
				Lat:          13.73183232872035,
				Lon:          100.64473371251563,
				Hgt:          7.468887195922434,
				Undulation:   -30.850798,
				DatumID:      DatumWGS84,
				LatSigma:     0.21609691,
				LonSigma:     0.25021353,
				HgtSigma:     1.8479676,
				StnID:        StationID{' ', ' ', ' ', ' '},
				DiffAge:      0,
				SolAge:       0,
				NumSVs:       29,
				NumSolnSVs:   29,
				NumSolnL1SVs: 29,
				NumSolnMulti: 29,
			},
			SinoPosFlags: SinoPosFlags{SigMask: 25},
		},
		fixupValueForAscii: func(msg MsgBody) MsgBody {
			r := *msg.(*SinoPsrPos)
			fixupPosForAscii(&r.Pos)
			return &r
		},
		fixupHeaderForAscii: sinoHdrForAscii,
	},
	{
		name:  "SinoGNSS K901 BESTXYZ SINGLE",
		hex:   "aa44121cf100022070000000b7b48509668c502100001000f6ff0100000000001000000002a02f1b7a7731c16a12b890983b5741a420c55dabf3364170b2373ee973013f36f3423e0000000008000000304b28f505c00dbfa1f59716f43e6b3f7a435d6e3b6c703f600a7e3d86cbd23da269763d0000000000000000000000000000803f1e1d1d1d00001f19395cc591",
		ascii: "#BESTXYZA,COM1,0,60.0,FINESTEERING,2437,558926.950,00000000,0000,1114;SOL_COMPUTED,SINGLE,-1144698.1062,6090338.2612,1504171.3663,0.1794,0.5057,0.1904,SOL_COMPUTED,DOPPLER_VELOCITY,-0.0001,0.0033,0.0040,0.0620,0.1029,0.0602,\"\",0.000,0.000,2.000,30,29,29,29,0,0,31,25*f87430bf\r\n",
		hdr:   sinoHdr(183, 558926950, 65526, 1),
		value: &SinoBestXYZ{
			XYZ: XYZ[SolStatus, SinoPosType]{
				PSolStatus:   SolComputed,
				PosType:      PosSingle,
				PX:           -1144698.1061954503,
				PY:           6090338.261234859,
				PZ:           1504171.366289177,
				PXSigma:      0.17939162,
				PYSigma:      0.5056749,
				PZSigma:      0.1903809,
				VSolStatus:   SolComputed,
				VelType:      PosDopplerVelocity,
				VX:           -0.0000567437952164934,
				VY:           0.003325916991115022,
				VZ:           0.00400946822431158,
				VXSigma:      0.062021613,
				VYSigma:      0.10292725,
				VZSigma:      0.060159333,
				StnID:        StationID{},
				SolAge:       1,
				NumSVs:       30,
				NumSolnSVs:   29,
				NumSolnL1SVs: 29,
				NumSolnMulti: 29,
			},
			SinoPosFlags: SinoPosFlags{SolStatFlag: 31, SigMask: 25},
		},
		fixupValueForAscii: func(msg MsgBody) MsgBody {
			r := *msg.(*SinoBestXYZ)
			fixupXYZForAscii(&r.XYZ)
			r.SolAge = 2
			return &r
		},
		fixupHeaderForAscii: sinoHdrForAscii,
	},
	{
		name:  "SinoGNSS K901 BESTVEL DOPPLER_VELOCITY",
		hex:   "aa44121c630002202c000000b8b485096a835021000010000000204e000000000800000000000000000000009af6b592f7515e3f5b5dcce4e8117240eeead780c473593f01008028898d4b4a",
		ascii: "#BESTVELA,COM1,0,60.0,FINESTEERING,2437,558924.650,00000000,0000,1114;SOL_COMPUTED,DOPPLER_VELOCITY,0.000,0.000,0.0019,289.119359,0.0016,0.0*896c6970\r\n",
		hdr:   sinoHdr(184, 558924650, 0, 20000),
		value: &BestVel{Vel: Vel[PosType]{
			SolStatus: SolComputed,
			VelType:   PosDopplerVelocity,
			HorSpd:    0.0018505971628139163,
			TrkGnd:    289.11935882406186,
			VertSpd:   0.0015534800508009666,
			Reserved:  1.4210856e-14,
		}},
		fixupValueForAscii: func(msg MsgBody) MsgBody {
			r := *msg.(*BestVel)
			fixupVelForAscii(&r.Vel)
			return &r
		},
		fixupHeaderForAscii: sinoHdrForAscii,
	},
	{
		name:  "SinoGNSS K901 PSRVEL DOPPLER_VELOCITY",
		hex:   "aa44121c640002202c000000b7b485095e9e5021000010000d00204e00000000080000000000000000000000e6b6d54902eb623f582192d4e8a5754068d159617779553f01009030c7380449",
		ascii: "#PSRVELA,COM1,0,60.0,FINESTEERING,2437,558931.550,00000000,0000,1114;SOL_COMPUTED,DOPPLER_VELOCITY,0.000,0.000,0.0023,346.369343,0.0013,0.0*cd9555a8\r\n",
		hdr:   sinoHdr(183, 558931550, 13, 20000),
		value: &SinoPsrVel{Vel: Vel[SinoPosType]{
			SolStatus: SolComputed,
			VelType:   PosDopplerVelocity,
			HorSpd:    0.0023093266196870686,
			TrkGnd:    346.3693433483327,
			VertSpd:   0.001310698110868003,
			Reserved:  1.047738e-09,
		}},
		fixupValueForAscii: func(msg MsgBody) MsgBody {
			r := *msg.(*SinoPsrVel)
			fixupVelForAscii(&r.Vel)
			return &r
		},
		fixupHeaderForAscii: sinoHdrForAscii,
	},
	{
		name:  "SinoGNSS K901 PSRDOP",
		hex:   "aa44121cae00022090000000b8b485095aa7502100001000020001003533a43f73b28c3f1d2d1d3f0d04673fe64d293f000020411d00000008000000040000000900000002000000030000000100000007000000940000008f0000008e000000950000009a000000b3000000a2000000a1000000af000000a900000026000000270000003b0000003c000000000000000000000000000000000000000000000000000000000000000000000009809eea",
		ascii: "#PSRDOPA,COM1,0,60.0,FINESTEERING,2437,558933.850,00000000,0000,1114;1.2828,1.0992,0.6140,0.9024,0.6613,10.0,29,8,4,9,2,3,1,7,148,143,142,149,154,179,162,161,175,169,38,39,59,60,0,0,0,0,0,0,0,0*718d4809\r\n",
		hdr:   sinoHdr(184, 558933850, 2, 1),
		value: &PsrDop{
			PsrDopInitChunk: PsrDopInitChunk{
				GDOP:    1.2828127,
				PDOP:    1.0991958,
				HDOP:    0.6139696,
				HTDOP:   0.90240556,
				TDOP:    0.6613449,
				Cutoff:  10,
				NumPRNs: 29,
			},
			PRNs: []PsrDopPRN{
				{8}, {4}, {9}, {2}, {3}, {1}, {7}, {148},
				{143}, {142}, {149}, {154}, {179}, {162}, {161}, {175},
				{169}, {38}, {39}, {59}, {60}, {0}, {0}, {0},
				{0}, {0}, {0}, {0}, {0},
			},
		},
		fixupValueForAscii:  fixupPsrDopForAscii,
		fixupHeaderForAscii: sinoHdrForAscii,
	},
}

// sinoBinCtors and sinoAsciiCtors are the registries with the SinoGNSS log
// types, as the SinoGNSS variant of the NovAtel packet processors uses.
func sinoBinCtors() map[MsgID]func() MsgBody {
	m := maps.Clone(BinRegistry())
	m[BestPosID] = func() MsgBody { return &SinoBestPos{} }
	m[PsrPosID] = func() MsgBody { return &SinoPsrPos{} }
	m[PsrVelID] = func() MsgBody { return &SinoPsrVel{} }
	m[BestXYZID] = func() MsgBody { return &SinoBestXYZ{} }
	return m
}

func sinoAsciiCtors() map[string]func() MsgBody {
	m := maps.Clone(AsciiRegistry())
	m["BESTPOSA"] = func() MsgBody { return &SinoBestPos{} }
	m["PSRPOSA"] = func() MsgBody { return &SinoPsrPos{} }
	m["PSRVELA"] = func() MsgBody { return &SinoPsrVel{} }
	m["BESTXYZA"] = func() MsgBody { return &SinoBestXYZ{} }
	return m
}

func TestSinoNavBinary(t *testing.T) {
	testDataBin(t, sinoNavTests, sinoBinCtors())
}

func TestSinoNavAscii(t *testing.T) {
	testDataAscii(t, sinoNavTests, sinoAsciiCtors())
}

// sinoHdr returns the binary header of a K901 log on COM1 in week 2437. The K901
// sets the message type byte, which its manual calls reserved, to 0x02.
func sinoHdr(idle Percentage, ms GPSec, reserved uint16, version uint16) MsgHdr[Port] {
	return MsgHdr[Port]{
		MessageType: 0x02,
		Port:        COM1,
		CommonHdr: CommonHdr{
			IdleTime:           idle,
			TimeStatus:         TimeStatusFineSteering,
			Week:               2437,
			MillisecondsOfWeek: ms,
			RecvStatus:         1048576,
			Reserved:           reserved,
			Version:            version,
		},
	}
}

// sinoHdrForAscii replaces the binary header fields that a SinoGNSS ASCII
// header gives fixed values.
func sinoHdrForAscii(hdr MsgHdr[Port]) MsgHdr[Port] {
	hdr.IdleTime = 120
	hdr.RecvStatus = 0
	hdr.Reserved = 0
	hdr.Version = 1114
	return hdr
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
	r := *msg.(*BestPos)
	fixupPosForAscii(&r.Pos)
	return &r
}

func fixupPosForAscii[S, P ~uint32](p *Pos[S, P]) {
	fixupFloat(&p.Lat, "%.11f")
	fixupFloat(&p.Lon, "%.11f")
	fixupFloat(&p.Hgt, "%.4f")
	fixupFloat32(&p.Undulation, "%.4f")
	fixupFloat32(&p.LatSigma, "%.4f")
	fixupFloat32(&p.LonSigma, "%.4f")
	fixupFloat32(&p.HgtSigma, "%.4f")
}

func fixupBestXYZForAscii(msg MsgBody) MsgBody {
	r := *msg.(*BestXYZ)
	fixupXYZForAscii(&r.XYZ)
	return &r
}

func fixupXYZForAscii[S, P ~uint32](x *XYZ[S, P]) {
	fixupFloat(&x.PX, "%.4f")
	fixupFloat(&x.PY, "%.4f")
	fixupFloat(&x.PZ, "%.4f")
	fixupFloat32(&x.PXSigma, "%.4f")
	fixupFloat32(&x.PYSigma, "%.4f")
	fixupFloat32(&x.PZSigma, "%.4f")
	fixupFloat(&x.VX, "%.4f")
	fixupFloat(&x.VY, "%.4f")
	fixupFloat(&x.VZ, "%.4f")
	fixupFloat32(&x.VXSigma, "%.4f")
	fixupFloat32(&x.VYSigma, "%.4f")
	fixupFloat32(&x.VZSigma, "%.4f")
}

func fixupBestGNSSVelForAscii(msg MsgBody) MsgBody {
	r := *msg.(*BestGNSSVel)
	fixupVelForAscii(&r.Vel)
	return &r
}

func fixupVelForAscii[P ~uint32](v *Vel[P]) {
	fixupFloat32(&v.Latency, "%.3f")
	fixupFloat32(&v.Age, "%.3f")
	fixupFloat(&v.HorSpd, "%.4f")
	fixupFloat(&v.TrkGnd, "%.6f")
	fixupFloat(&v.VertSpd, "%.4f")
	fixupFloat32(&v.Reserved, "%.1f")
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
