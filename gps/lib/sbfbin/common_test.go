package sbfbin

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func TestCRC16(t *testing.T) {
	if got, want := CRC16([]byte("123456789")), uint16(0x31C3); got != want {
		t.Fatalf("CRC16 = 0x%04X, want 0x%04X", got, want)
	}
}

func TestMsgID(t *testing.T) {
	mid := MsgID(PVTGeodeticID) | 3<<13
	n, rev := mid.Unpack()
	if n != uint16(PVTGeodeticID) || rev != 3 {
		t.Fatalf("Unpack = (%d, %d), want (%d, 3)", n, rev, PVTGeodeticID)
	}
	if got := mid.String(); got != "PVTGeodetic" {
		t.Fatalf("String = %q, want %q", got, "PVTGeodetic")
	}
}

func TestOtherBlockNumbers(t *testing.T) {
	tests := []struct {
		id   int
		name string
	}{
		{GPSNavID, "GPSNav"},
		{GPSIonID, "GPSIon"},
		{GEONavID, "GEONav"},
		{GPSCNavID, "GPSCNav"},
		{GALIonID, "GALIon"},
		{BDSIonID, "BDSIon"},
		{BDSCNav1ID, "BDSCNav1"},
		{BDSCNav2ID, "BDSCNav2"},
		{BDSCNav3ID, "BDSCNav3"},
		{NavICLNavID, "NavICLNav"},
		{GEOServiceLevelID, "GEOServiceLevel"},
		{GEOClockEphCovMatrixID, "GEOClockEphCovMatrix"},
		{GEOIGPMaskID, "GEOIGPMask"},
	}
	want := map[string]int{
		"GPSNav":               5891,
		"GPSIon":               5893,
		"GEONav":               5896,
		"GPSCNav":              4042,
		"GALIon":               4030,
		"BDSIon":               4120,
		"BDSCNav1":             4251,
		"BDSCNav2":             4252,
		"BDSCNav3":             4253,
		"NavICLNav":            4254,
		"GEOServiceLevel":      5917,
		"GEOClockEphCovMatrix": 5934,
		"GEOIGPMask":           5931,
	}
	seen := make(map[int]string)
	for _, tt := range tests {
		if tt.id != want[tt.name] {
			t.Fatalf("%s = %d, want %d", tt.name, tt.id, want[tt.name])
		}
		if prev := seen[tt.id]; prev != "" {
			t.Fatalf("%s collides with %s at block %d", tt.name, prev, tt.id)
		}
		seen[tt.id] = tt.name
		if got := MsgID(tt.id).String(); got != tt.name {
			t.Fatalf("MsgID(%d).String() = %q, want %q", tt.id, got, tt.name)
		}
	}
}

func testBlock(t *testing.T, b *Block) *Block {
	t.Helper()
	pkt, err := Serialize(b)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseMsg(string(pkt))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, b) {
		t.Fatalf("round trip mismatch:\ngot  %#v\nwant %#v", got, b)
	}
	return got
}

// TestMeasExtraManyChannels checks that a MeasExtra with 256 channel
// sub-blocks round-trips. MeasExtra.N is a u1 that wraps to 0 at 256, so the
// channel count must come from Channels on write and from the payload length
// on read, never from N.
func TestMeasExtraManyChannels(t *testing.T) {
	ts := TimeStamp{TOW: 0x11223344, WNc: 0x5566}
	chans := make([]MeasExtraChannelSub, 256)
	for i := range chans {
		chans[i].measExtraBase = measExtraBase{RxChannel: uint8(i), Type: 3, LockTime: uint16(i)}
	}
	pkt, err := Serialize(&Block{Rev: 3, TimeStamp: ts, Params: &MeasExtra{
		measExtraHead: measExtraHead{DopplerVarFactor: 1.5},
		Channels:      chans,
	}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseMsg(string(pkt))
	if err != nil {
		t.Fatal(err)
	}
	gotChans := got.Params.(*MeasExtra).Channels
	if len(gotChans) != 256 {
		t.Fatalf("round trip produced %d channels, want 256 (N wraps to 0 at 256)", len(gotChans))
	}
	if !reflect.DeepEqual(gotChans, chans) {
		t.Fatal("channel data changed across round trip")
	}
}

func TestRoundTripM1Blocks(t *testing.T) {
	ts := TimeStamp{TOW: 0x11223344, WNc: 0x5566}
	testBlock(t, &Block{Rev: 0, TimeStamp: ts, Params: &EndOfMeas{}})
	testBlock(t, &Block{Rev: 2, TimeStamp: ts, Params: &PVTGeodetic{
		pvtGeodeticFixed: pvtGeodeticFixed{
			Mode:        ModeRTKFixed,
			Error:       ErrNone,
			Latitude:    0.2401,
			Longitude:   1.7132,
			Height:      123.5,
			Undulation:  12.25,
			Vn:          1.25,
			Ve:          -2.5,
			Vu:          0.5,
			COG:         90.25,
			RxClkBias:   -0.001,
			RxClkDrift:  0.002,
			TimeSystem:  TimeSystemGPS,
			Datum:       DatumWGS84,
			NrSV:        14,
			WACorrInfo:  0x23,
			ReferenceID: 120,
			MeanCorrAge: 42,
			SignalInfo:  0x01020304,
			AlertFlag:   0x60,
			NrBases:     2,
		},
		pvtTrailer: pvtTrailer{
			pvtRev1: pvtRev1{PPPInfo: 0x4001, Latency: 25},
			pvtRev2: pvtRev2{HAccuracy: 12, VAccuracy: 34, Misc: 0x80},
		},
	}})
	testBlock(t, &Block{Rev: 3, TimeStamp: ts, Params: &MeasExtra{
		measExtraHead: measExtraHead{DopplerVarFactor: 1.5},
		Channels: []MeasExtraChannelSub{
			{
				measExtraBase: measExtraBase{
					RxChannel:     1,
					Type:          3,
					MPCorrection:  -4,
					SmoothingCorr: 5,
					CodeVar:       12,
					CarrierVar:    13,
					LockTime:      14,
					CumLossCont:   15,
				},
				measExtraRev1: measExtraRev1{CarMPCorr: -2},
				measExtraRev2: measExtraRev2{Info: 3},
				measExtraRev3: measExtraRev3{Misc: 4},
			},
		},
	}})
	testBlock(t, &Block{Rev: 1, TimeStamp: ts, Params: &MeasEpoch{
		measEpochHead: measEpochHead{CommonFlags: 0x41, CumClkJumps: 7},
		Type1: []MeasEpochChannelType1{
			{
				RxChannel:  1,
				Type:       3,
				SVID:       22,
				Misc:       1,
				CodeLSB:    123456,
				Doppler:    -123,
				CarrierLSB: 17,
				CarrierMSB: 2,
				CN0:        180,
				LockTime:   99,
				ObsInfo:    4,
			},
		},
		Type2: [][]MeasEpochChannelType2{{
			{
				Type:             4,
				LockTime:         5,
				CN0:              160,
				OffsetsMSB:       0x39,
				CarrierMSB:       -3,
				ObsInfo:          2,
				CodeOffsetLSB:    100,
				CarrierLSB:       200,
				DopplerOffsetLSB: 300,
			},
		}},
	}})
}

func TestPVTGeodeticRevisionTolerance(t *testing.T) {
	ts := TimeStamp{TOW: 1000, WNc: 2000}
	p := &PVTGeodetic{
		pvtGeodeticFixed: pvtGeodeticFixed{
			Mode:       ModeStandalone,
			Latitude:   1,
			Longitude:  2,
			Height:     3,
			TimeSystem: TimeSystemGPS,
			Datum:      DatumWGS84,
			NrSV:       10,
		},
	}
	p.setDNUDefaults()
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, Endian, ts); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(buf, Endian, &p.pvtGeodeticFixed); err != nil {
		t.Fatal(err)
	}
	pkt, err := PackMsg(PVTGeodeticID, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseMsg(string(pkt))
	if err != nil {
		t.Fatal(err)
	}
	gp := got.Params.(*PVTGeodetic)
	if gp.Latency != PVTAccuracyDNU || gp.HAccuracy != PVTAccuracyDNU || gp.VAccuracy != PVTAccuracyDNU {
		t.Fatalf("revision trailer defaults = (%d, %d, %d)", gp.Latency, gp.HAccuracy, gp.VAccuracy)
	}
	if got.Rev != 0 {
		t.Fatalf("Rev = %d, want 0", got.Rev)
	}
	pkt, err = Serialize(got)
	if err != nil {
		t.Fatal(err)
	}
	if _, rev := MsgID(Endian.Uint16(pkt[4:6])).Unpack(); rev != 2 {
		t.Fatalf("Serialize of rev-0 block stamped rev %d, want 2", rev)
	}
	buf.Write([]byte{1, 2, 3, 4})
	pkt, err = PackMsg(PVTGeodeticID|2<<13, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseMsg(string(pkt)); err != nil {
		t.Fatalf("future trailing bytes were rejected: %v", err)
	}
}

func TestRequiredFieldsCannotBeTruncated(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, Endian, TimeStamp{TOW: 1, WNc: 2}); err != nil {
		t.Fatal(err)
	}
	pkt, err := PackMsg(DOPID, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseMsg(string(pkt)); err == nil {
		t.Fatalf("truncated DOP parsed successfully")
	}
}

func TestPackMsgDoesNotModifyPayloadBackingArray(t *testing.T) {
	buf := []byte{1, 2, 3, 0xAA, 0xBB}
	if _, err := PackMsg(EndOfMeasID, buf[:3]); err != nil {
		t.Fatal(err)
	}
	if buf[3] != 0xAA || buf[4] != 0xBB {
		t.Fatalf("PackMsg modified caller backing array: %x", buf)
	}
}

func TestSerializeRejectsOversizeOneByteCounts(t *testing.T) {
	tests := []struct {
		name string
		p    Params
	}{
		{
			name: "meas epoch outer",
			p: &MeasEpoch{
				Type1: make([]MeasEpochChannelType1, 256),
				Type2: make([][]MeasEpochChannelType2, 256),
			},
		},
		{
			name: "meas epoch inner",
			p: &MeasEpoch{
				Type1: []MeasEpochChannelType1{{}},
				Type2: [][]MeasEpochChannelType2{make([]MeasEpochChannelType2, 256)},
			},
		},
		{
			name: "rf status",
			p:    &RFStatus{RFBand: make([]RFBand, 256)},
		},
		{
			name: "receiver status",
			p:    &ReceiverStatus{AGCState: make([]AGCState, 256)},
		},
		{
			name: "sat visibility",
			p:    &SatVisibility{SatInfo: make([]SatInfo, 256)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Serialize(&Block{Params: tt.p})
			if err == nil {
				t.Fatalf("Serialize succeeded")
			}
		})
	}
}

func TestMeasEpochSBLengthTolerance(t *testing.T) {
	b := &Block{
		TimeStamp: TimeStamp{TOW: 1, WNc: 2},
		Params: &MeasEpoch{
			measEpochHead: measEpochHead{SB1Length: 24, SB2Length: 16},
			Type1: []MeasEpochChannelType1{
				{RxChannel: 1, Type: 3, SVID: 10},
				{RxChannel: 2, Type: 4, SVID: 11},
			},
			Type2: [][]MeasEpochChannelType2{{
				{Type: 5, LockTime: 6, CN0: 7},
			}, nil},
		},
	}
	got := testBlock(t, b)
	m := got.Params.(*MeasEpoch)
	if len(m.Type1) != 2 || len(m.Type2) != 2 || len(m.Type2[0]) != 1 {
		t.Fatalf("bad sub-block counts: %d %d %d", len(m.Type1), len(m.Type2), len(m.Type2[0]))
	}
	if m.SB1Length != 24 || m.SB2Length != 16 {
		t.Fatalf("SBLength = (%d, %d), want (24, 16)", m.SB1Length, m.SB2Length)
	}
}

func TestRoundTripM2Blocks(t *testing.T) {
	ts := TimeStamp{TOW: 100, WNc: 200}
	testBlock(t, &Block{TimeStamp: ts, Params: &EndOfPVT{}})
	testBlock(t, &Block{TimeStamp: ts, Params: &DOP{
		NrSV: 12,
		PDOP: 101,
		TDOP: 102,
		HDOP: 103,
		VDOP: 104,
		HPL:  5.5,
		VPL:  6.5,
	}})
	testBlock(t, &Block{Rev: 2, TimeStamp: ts, Params: &PVTCartesian{
		pvtCartesianFixed: pvtCartesianFixed{
			Mode:        ModeDifferential,
			X:           1,
			Y:           2,
			Z:           3,
			Undulation:  4,
			Vx:          5,
			Vy:          6,
			Vz:          7,
			COG:         8,
			RxClkBias:   9,
			RxClkDrift:  10,
			TimeSystem:  TimeSystemGPS,
			Datum:       DatumWGS84,
			NrSV:        11,
			WACorrInfo:  12,
			ReferenceID: 13,
			MeanCorrAge: 14,
			SignalInfo:  15,
			AlertFlag:   16,
			NrBases:     17,
		},
		pvtTrailer: pvtTrailer{
			pvtRev1: pvtRev1{PPPInfo: 18, Latency: 19},
			pvtRev2: pvtRev2{HAccuracy: 20, VAccuracy: 21, Misc: 22},
		},
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &PosCovCartesian{posCovCartesian{
		Mode: ModeRTKFixed, Cov_xx: 1, Cov_yy: 2, Cov_zz: 3, Cov_bb: 4,
		Cov_xy: 5, Cov_xz: 6, Cov_xb: 7, Cov_yz: 8, Cov_yb: 9, Cov_zb: 10,
	}}})
	testBlock(t, &Block{TimeStamp: ts, Params: &PosCovGeodetic{posCovGeodetic{
		Mode: ModeRTKFloat, Cov_latlat: 1, Cov_lonlon: 2, Cov_hgthgt: 3, Cov_bb: 4,
		Cov_latlon: 5, Cov_lathgt: 6, Cov_latb: 7, Cov_lonhgt: 8, Cov_lonb: 9, Cov_hb: 10,
	}}})
	testBlock(t, &Block{TimeStamp: ts, Params: &VelCovCartesian{velCovCartesian{
		Mode: ModeSBAS, Cov_VxVx: 1, Cov_VyVy: 2, Cov_VzVz: 3, Cov_DtDt: 4,
		Cov_VxVy: 5, Cov_VxVz: 6, Cov_VxDt: 7, Cov_VyVz: 8, Cov_VyDt: 9, Cov_VzDt: 10,
	}}})
	testBlock(t, &Block{TimeStamp: ts, Params: &VelCovGeodetic{velCovGeodetic{
		Mode: ModePPP, Cov_VnVn: 1, Cov_VeVe: 2, Cov_VuVu: 3, Cov_DtDt: 4,
		Cov_VnVe: 5, Cov_VnVu: 6, Cov_VnDt: 7, Cov_VeVu: 8, Cov_VeDt: 9, Cov_VuDt: 10,
	}}})
	testBlock(t, &Block{TimeStamp: ts, Params: &QualityInd{Indicators: []uint16{0x0A00, 0x1501}}})
	testBlock(t, &Block{TimeStamp: ts, Params: &GALAuthStatus{
		OSNMAStatus:      0x1234,
		TrustedTimeDelta: 1.25,
		GalActiveMask:    0x0102030405060708,
		GalAuthenticMask: 0x1112131415161718,
		GpsActiveMask:    0x2122232425262728,
		GpsAuthenticMask: 0x3132333435363738,
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &RFStatus{
		rfStatusHead: rfStatusHead{Flags: 3},
		RFBand: []RFBand{{
			Frequency: 1575420000,
			Bandwidth: 20000,
			Info:      0x81,
			Power:     -12,
		}},
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &ReceiverStatus{
		receiverStatusHead: receiverStatusHead{
			CPULoad:     50,
			ExtError:    2,
			UpTime:      12345,
			RxState:     0x10020,
			RxError:     0x800,
			CmdCount:    9,
			Temperature: 130,
		},
		AGCState: []AGCState{{FrontEndID: 1, Gain: -2, SampleVar: 100, BlankingStat: 3}},
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &SatVisibility{
		SatInfo: []SatInfo{{
			satInfoBase: satInfoBase{
				SVID: 10, FreqNr: 8, Azimuth: 12345, Elevation: 6789,
				RiseSet: 1, SatelliteInfo: SatelliteInfoEphemeris,
			},
			satInfoRev1: satInfoRev1{SVIDFull: 10},
		}},
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &ChannelStatus{
		SatInfo: []ChannelSatInfo{{
			SVID: 22, FreqNr: 8, AzimuthRiseSet: 0x4001, HealthStatus: 0xAAAA,
			Elevation: 45, RxChannel: 3,
		}},
		StateInfo: [][]ChannelStateInfo{{
			{Antenna: 1, TrackingStatus: 0x5555, PVTStatus: 0xAAAA, PVTInfo: 7},
		}},
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &ReceiverTime{
		UTCYear: 26, UTCMonth: 7, UTCDay: 3, UTCHour: 12,
		UTCMin: 34, UTCSec: 56, DeltaLS: 18, SyncLevel: 7,
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &XPPSOffset{SyncAge: 1, Timescale: PPSTimescaleGPS, Offset: -3.5}})
	setup := &ReceiverSetup{}
	setup.setDNUDefaults()
	copy(setup.MarkerName[:], "marker")
	copy(setup.MarkerNumber[:], "number")
	copy(setup.Observer[:], "observer")
	copy(setup.Agency[:], "agency")
	copy(setup.RxSerialNumber[:], "serial")
	copy(setup.RxName[:], "rx")
	copy(setup.RxVersion[:], "version")
	copy(setup.AntSerialNbr[:], "antserial")
	copy(setup.AntType[:], "anttype")
	setup.DeltaH = 1
	setup.DeltaE = 2
	setup.DeltaN = 3
	copy(setup.MarkerType[:], "GEODETIC")
	copy(setup.GNSSFWVersion[:], "fw")
	copy(setup.ProductName[:], "mosaic")
	setup.Latitude = 0.1
	setup.Longitude = 0.2
	setup.Height = 0.3
	copy(setup.StationCode[:], "STAT")
	setup.MonumentIdx = 4
	setup.ReceiverIdx = 5
	copy(setup.CountryCode[:], "THA")
	testBlock(t, &Block{Rev: 4, TimeStamp: ts, Params: setup})
	testBlock(t, &Block{TimeStamp: ts, Params: &GPSUtc{
		PRN: 1, A_1: 0.1, A_0: 0.2, T_ot: 345600, WN_t: 10,
		DEL_t_LS: 18, WN_LSF: 20, DN: 7, DEL_t_LSF: 19,
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &GALUtc{
		SVID: 71, Source: GALUtcSourceINAV, A_1: 0.1, A_0: 0.2, T_ot: 345600,
		WN_ot: 10, DEL_t_LS: 18, WN_LSF: 20, DN: 7, DEL_t_LSF: 19,
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &BDSUtc{
		PRN: 141, A_1: 0.1, A_0: 0.2, DEL_t_LS: 4, WN_LSF: 20, DN: 6, DEL_t_LSF: 5,
	}})
}

func TestReceiverSetupRevisionTolerance(t *testing.T) {
	ts := TimeStamp{TOW: 1, WNc: 2}
	rs := &ReceiverSetup{}
	rs.setDNUDefaults()
	copy(rs.MarkerName[:], "marker")
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, Endian, ts); err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(buf, Endian, &rs.receiverSetupFixed); err != nil {
		t.Fatal(err)
	}
	pkt, err := PackMsg(ReceiverSetupID, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseMsg(string(pkt))
	if err != nil {
		t.Fatal(err)
	}
	gp := got.Params.(*ReceiverSetup)
	if gp.Latitude != ReceiverSetupPositionDNU || gp.Longitude != ReceiverSetupPositionDNU || gp.Height != ReceiverSetupPositionDNU {
		t.Fatalf("receiver setup defaults = (%v, %v, %v)", gp.Latitude, gp.Longitude, gp.Height)
	}
	pkt, err = Serialize(got)
	if err != nil {
		t.Fatal(err)
	}
	if _, rev := MsgID(Endian.Uint16(pkt[4:6])).Unpack(); rev != 4 {
		t.Fatalf("Serialize of rev-0 block stamped rev %d, want 4", rev)
	}
}

func TestRoundTripTier2Blocks(t *testing.T) {
	ts := TimeStamp{TOW: 300, WNc: 400}
	testBlock(t, &Block{TimeStamp: ts, Params: &DiffCorrIn{
		diffCorrInHead: diffCorrInHead{Mode: DiffCorrModeSPARTN, Source: 9},
		Correction:     []byte{0x73, 0x01, 0x02, 0x03, 0x00, 0x00, 0x00, 0x00},
	}})
	testBlock(t, &Block{TimeStamp: ts, Params: &BaseStation{
		BaseStationID: 100,
		BaseType:      BaseTypeFixed,
		Source:        8,
		Datum:         DatumWGS84,
		X:             1.25,
		Y:             2.5,
		Z:             3.75,
	}})
}
