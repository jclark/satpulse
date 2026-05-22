package rtcmbin

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/lib/bitsenc"
)

func TestParseMT1005(t *testing.T) {
	result, err := ParseMsg(rtcm1005)
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := result.(*MT1005)
	if !ok {
		t.Fatalf("ParseMsg returned %T, want *MT1005", result)
	}
	if msg.MsgNum != 1005 {
		t.Errorf("MsgNum = %d, want 1005", msg.MsgNum)
	}
	if msg.MsgType() != 1005 {
		t.Errorf("MsgType() = %d, want 1005", msg.MsgType())
	}
	// Cross-check with ReferenceStationID extraction
	wantID, _ := ReferenceStationID([]byte(rtcm1005))
	if msg.StationID != wantID {
		t.Errorf("StationID = %d, want %d", msg.StationID, wantID)
	}
	if !msg.GPS {
		t.Error("GPS = false, want true")
	}
	ecef := msg.ECEF()
	t.Logf("MT1005: station=%d ITRF=%d GPS=%v GLO=%v GAL=%v ref=%v",
		msg.StationID, msg.ITRFYear, msg.GPS, msg.GLONASS, msg.Galileo, msg.RefStation)
	t.Logf("ECEF: X=%.4f Y=%.4f Z=%.4f m", ecef[0], ecef[1], ecef[2])
}

func TestParseMT1005Real(t *testing.T) {
	pkt, err := hex.DecodeString("d300133ed4d203bd55b51d7f8e2e1f5ad403808e27daf56b52")
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseMsg(string(pkt))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := result.(*MT1005)
	if !ok {
		t.Fatalf("ParseMsg returned %T, want *MT1005", result)
	}
	if msg.StationID != 1234 {
		t.Errorf("StationID = %d, want 1234", msg.StationID)
	}
	// Cross-check with ReferenceStationID extraction
	wantID, idOK := ReferenceStationID(pkt)
	if !idOK {
		t.Fatal("ReferenceStationID returned false")
	}
	if msg.StationID != wantID {
		t.Errorf("StationID = %d, want %d (from ReferenceStationID)", msg.StationID, wantID)
	}
	ecef := msg.ECEF()
	t.Logf("MT1005: station=%d ITRF=%d GPS=%v GLO=%v GAL=%v ref=%v singleOsc=%v qc=%d",
		msg.StationID, msg.ITRFYear, msg.GPS, msg.GLONASS, msg.Galileo, msg.RefStation,
		msg.SingleOsc, msg.QuarterCycle)
	t.Logf("ECEF: X=%.4f Y=%.4f Z=%.4f m", ecef[0], ecef[1], ecef[2])
}

func TestRoundTripMT1005(t *testing.T) {
	pkts := []struct {
		name string
		hex  string
	}{
		{"station 2003", hex.EncodeToString([]byte(rtcm1005))},
		{"station 1234", "d300133ed4d203bd55b51d7f8e2e1f5ad403808e27daf56b52"},
	}
	for _, tt := range pkts {
		t.Run(tt.name, func(t *testing.T) {
			pkt := decodePkt(t, tt.hex)
			payload := []byte(pkt[3 : len(pkt)-3])
			var msg MT1005
			if err := bitsenc.Read(payload, &msg); err != nil {
				t.Fatal(err)
			}
			got, err := bitsenc.Write(&msg)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Errorf("round-trip mismatch:\n got %X\nwant %X", got, payload)
			}
		})
	}
}

func TestParseMsgUnknown(t *testing.T) {
	pkt := "\xD3\x00\x01\x00\x00\x00\x00\x00"
	result, err := ParseMsg(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.(*UnknownMsg); !ok {
		t.Errorf("ParseMsg returned %T, want *UnknownMsg", result)
	}
}

func TestParseMT1230(t *testing.T) {
	pkt := decodePkt(t, "d3000c4ce4d28f000000000000000073929d")
	result, err := ParseMsg(pkt)
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := result.(*MT1230)
	if !ok {
		t.Fatalf("ParseMsg returned %T, want *MT1230", result)
	}
	if msg.StationID != 1234 {
		t.Errorf("StationID = %d, want 1234", msg.StationID)
	}
	if !msg.Indicator {
		t.Error("Indicator = false, want true")
	}
	if msg.SignalMask != 0x0F {
		t.Errorf("SignalMask = 0x%X, want 0x0F", msg.SignalMask)
	}
	if len(msg.CodePhaseBias) != 4 {
		t.Fatalf("CodePhaseBias len = %d, want 4", len(msg.CodePhaseBias))
	}
	for i, v := range msg.CodePhaseBias {
		if v != 0 {
			t.Errorf("CodePhaseBias[%d] = %d, want 0", i, v)
		}
	}
	// Cross-check station ID
	wantID, idOK := ReferenceStationID([]byte(pkt))
	if !idOK {
		t.Fatal("ReferenceStationID returned false")
	}
	if msg.StationID != wantID {
		t.Errorf("StationID = %d, want %d", msg.StationID, wantID)
	}
}

func decodePkt(t *testing.T, h string) string {
	t.Helper()
	b, err := hex.DecodeString(h)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMSMHeaderMethods(t *testing.T) {
	h := MSMHeader{MsgHdrStationID: MsgHdrStationID{MsgHdr: MsgHdr{MsgNum: 1074}}}
	if h.GNSS() != GPS {
		t.Errorf("GNSS() = %q, want GPS", h.GNSS())
	}
	if h.MSMLevel() != 4 {
		t.Errorf("MSMLevel() = %d, want 4", h.MSMLevel())
	}
	h.MsgNum = 1094
	if h.GNSS() != GALILEO {
		t.Errorf("GNSS() = %q, want GALILEO", h.GNSS())
	}
	h.MsgNum = 1124
	if h.GNSS() != BEIDOU {
		t.Errorf("GNSS() = %q, want BEIDOU", h.GNSS())
	}
	if h.MSMLevel() != 4 {
		t.Errorf("MSMLevel() = %d, want 4", h.MSMLevel())
	}
}

func TestMSMHeaderGNSSAll(t *testing.T) {
	tests := []struct {
		mt   MsgType
		want GNSS
	}{
		{1074, GPS},
		{1084, GLONASS},
		{1094, GALILEO},
		{1104, SBAS},
		{1114, QZSS},
		{1124, BEIDOU},
		{1134, IRNSS},
	}
	for _, tt := range tests {
		h := MSMHeader{MsgHdrStationID: MsgHdrStationID{MsgHdr: MsgHdr{MsgNum: tt.mt}}}
		if got := h.GNSS(); got != tt.want {
			t.Errorf("GNSS(%d) = %q, want %q", tt.mt, got, tt.want)
		}
	}
}

// Real MSM4 packets.
var msmPackets = []struct {
	name string
	hex  string
	mt   MsgType
	gnss GNSS
}{
	{"GPS MSM4", "d300a84324d2088045020020041d5c40000000002020010063b6d3fa88948e969ea29294889beeeef5e483f002a68678f0f05e797e480f830c5023c301fe35ecd0da9e08af144bf583cf77ba3f6c9ec7fe467c6963afce7e011e200cef80ca3883c1700b3e178e0bfe244d7aad51ebf8884470813909400001fb0407dd0dbfdb4b7f4f2ff9a1b0011e3e4431f8f73bff9ffffff541ffffff94000016dcde1655347138d336d784f1d4d300d2d0f5", 1074, GPS},
	{"GLONASS MSM4", "d3005543c4d20b1259c20020418c0100000000002080000057d252320a42525402d13db3af99f46098e8de7690f428dd4209c5c7b077f834f5f31f208abce27ae704c8d617b7a05c06d000007dfffff00054f638e5844d00b1eb60", 1084, GLONASS},
	{"Galileo MSM4", "d300a64464d20880450200200184043200000000200101007ffffd595551496145747502516849affdc37f893f8dbfc56853d235a9075e76e01de8433a075b900e0a5022184f516f93094668b0d8e520cd8a004d100722002273f9cba3ec1957aa08feadc77b47ddee10c049692146fc852582064ef80c7ee06f0706a0e81c931871a55e0d30f71845da5c07ffd7f5ee7fffffffff7f800002ebec7a179d7dbb71ba191dbf0b5e9620991956", 1094, GALILEO},
	{"QZSS MSM4", "d3004d45a4d20880450200203100000000000000200041007bdde1deba1a0e58f299f19fe188c0f1bf5842b1e1e30afc791bfaeb2feb7560000007197fbe649eff56fbfe92fff0afff00b2bb57665beeeabdac", 1114, QZSS},
	{"BeiDou MSM4 (1)", "d300dc4644d2087f6a42002037f80000000000002082014171c71c71c71c7043c3bbf42bbbc413ba6aa1ef0f6aada915f0ea59a47178141829a64d1c94c93542493f423f187d223f7685f10a96c81d869aeb7f3f7e90fbe42f5c6878d0b30bc6254c1cfa9ff61be8ff24a84dbca1338f04be8c1233404804811b25fef14dfb923fed3ec16d480531c8141017b5ce5ee0277b82da01ded80c8ee0393900bad0019537fa65e195a405a6001683cfed95ffb2987ead47e4603ffffffffffffffffffffffffe5d48000000598e37639575d4f3f35d6db4d76cd3cf6595b8fb8e00a95dd9", 1124, BEIDOU},
}

func TestRoundTripMSM(t *testing.T) {
	for _, tt := range msmPackets {
		t.Run(tt.name, func(t *testing.T) {
			pkt := decodePkt(t, tt.hex)
			payload := []byte(pkt[3 : len(pkt)-3])
			var msg MSM
			if err := bitsenc.NewReader(payload).Read(&msg); err != nil {
				t.Fatal(err)
			}
			got, err := bitsenc.Write(&msg)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Errorf("round-trip mismatch:\n got %X\nwant %X", got, payload)
			}
		})
	}
}

// Legacy observable captures from the Unicore UM982 (rtcm-legacy.jsonl).
const (
	rtcm1001Um982 = "d3004a3e900012a001629010b60b9ffe2ddfc65f0ef300079ff1d253a93ff8bbfc9261c3e7fb6b7f40ea8e10040e5fc513ab0940000007889f24c01ee7fcb38b19afda9aff550df337ef94db0073cf7b"
	rtcm1002Um982 = "d3005c3ea00012a001629010b60b9ffe2ddfd4a2465f0ef300079ff4e971d253a93ff8bbfd1b249261c3e7fb6b7f48c740ea8e10040e5fd46b8513ab0940000004c667889f24c01ee7fd1f00b38b19afda9aff489f550df337ef94db11678018910b"
	rtcm1003Um982 = "d3007a3eb00012a001629010b60b9ffe2ddfe0533fb8dbf8cbe1de6000f3ff7de1ffc5bfc7494ea4ffe2effbf30000b5fe4930e1f3fdb5bfdfbcffba4ff40ea8e10040e5fefd3403d47f8a275612800000040010000001e227c93007b9ffbfb001575fe59c58cd7ed4d7fdf6f007627f550df337ef94db2fabbf967c788cfcdb"
	rtcm1004Um982 = "d300953ec00012a001629010b60b9ffe2ddfd4a260533fb8dbfbd8cbe1de6000f3fe9d2f7de1ffc5bfe607494ea4ffe2eff46c9bf30000b5ff5c4930e1f3fdb5bfa463dfbcffba4ffb340ea8e10040e5fd46bafd3403d47fcc8a275612800000098cc4001000000001e227c93007b9ff47c0bfb001575ff6459c58cd7ed4d7fa44fdf6f007627f99550df337ef94db1167afabbf967c7bc887a49a"
	rtcm1009Um982 = "d300383f100004c5310301a2d04267fe08d7f983387dc5409a63fa653976e0a00000006605aa78003797f886da533efd8f2ff9c03b1fa580465ff8013f70"
	rtcm1010Um982 = "d300433f200004c5310301a2d04267fe08d7fa2b630670fb8a8134c7f4d3e994e5db8280000008d943302d53c001bcbfd15f886da533efd8f2ffa0ba380763f4b008cbff499aebcea3"
	rtcm1011Um982 = "d300583f300004c5310301a2d04267fe08d7f9000400000030670fb8a8134c7f7f39025b0fe994e5db828000000400100000003302d53c001bcbfdf9680364ff886da533efd8f2ffbf3780e8f7f380763f4b008cbff7f570197efe08bd26"
	rtcm1012Um982 = "d3006a3f400004c5310301a2d04267fe08d7fa2b6200080000000060ce1f71502698fe9a7dfce4096c3fd12653976e0a0000002365200080000000019816a9e000de5fe8afdf9680364ffdc886da533efd8f2ffa0ba7e6f01d1eff40700ec7e9601197fe9335fd5c065fbfd980154a88"
)

// Captures from the Centipede Ntrip network (French CORS).  An independent
// second source for MT1004/MT1012 that catches encoder-specific bugs a
// self-consistent round-trip would miss, and the only source of MT1013.
const (
	rtcm1004Centipede = "d300a53ec0001220afa2a1104808861b6e7fd3ab3fbe457157fca8a9fce6b02d3dfea15dfdfe02bb3feb863825cc024e7ff4abeff37061c7ff7c38b261c023733fa3e0ff6006377ffc12492abd819661fd170ffc9c435bbfe1163b514e04481fe8f85fde408e37fec90383e2400bd97f50a6fea9025b3fee4aa20a8ec16b43fa45f7f6384a39ffaf7570e291fbe81fd4e93fc43f36bbfd23cbbdc0e04d35fe9579feaa0bd1dfef00d7d7a4"
	rtcm1012Centipede = "d300ab3f400000caa3050841a34246046343fa7ab40581bea1ff26199c894550913bfe8305fe2436467fdf886d7d96dc1883ffa1bf7fab0bd94ff7a2901c507a01b09dea26d01dc0c8337c618365bde0015e8ffa4b2404007219ff5468a8c6a0003481fe8e988002000000001c0181309c0794bfa7af408703b2aff429089a61c4049f7fe9691012815bc3fd12651a3ce9c0b3d3fa2bd7f870499cff70a1288f94900b72fe96c9ffe402d9ffd60049ca77"
	rtcm1013Centipede = "d300093f5000eed69479004898988b"
)

// legacyObsSpot is a spot-check summary of a parsed legacy-observable
// message.  Keeping the fields small lets the tests compare the whole
// struct with reflect.DeepEqual instead of field-by-field assertions.
type legacyObsSpot struct {
	MsgType    MsgType
	StationID  uint16
	Nsat       uint8
	SatLen     int
	FirstSatID uint8
}

// spotCheck pulls the fields of legacyObsSpot out of any legacy observable
// message via reflection, so a single table-driven test can cover the
// eight per-sat message types.
func spotCheck(msg Msg) legacyObsSpot {
	out := legacyObsSpot{MsgType: msg.MsgType()}
	if s, ok := msg.(StationIDer); ok {
		out.StationID = s.RefStationID()
	}
	v := reflect.ValueOf(msg).Elem()
	if nsat := v.FieldByName("Nsat"); nsat.IsValid() {
		out.Nsat = uint8(nsat.Uint())
	}
	if sat := v.FieldByName("Sat"); sat.IsValid() {
		out.SatLen = sat.Len()
		if sat.Len() > 0 {
			out.FirstSatID = uint8(sat.Index(0).FieldByName("SatID").Uint())
		}
	}
	return out
}

func TestParseLegacyObservables(t *testing.T) {
	tests := []struct {
		name   string
		hex    string
		expect legacyObsSpot
	}{
		{"MT1001 UM982", rtcm1001Um982, legacyObsSpot{1001, 0, 9, 9, 4}},
		{"MT1002 UM982", rtcm1002Um982, legacyObsSpot{1002, 0, 9, 9, 4}},
		{"MT1003 UM982", rtcm1003Um982, legacyObsSpot{1003, 0, 9, 9, 4}},
		{"MT1004 UM982", rtcm1004Um982, legacyObsSpot{1004, 0, 9, 9, 4}},
		{"MT1004 Centipede", rtcm1004Centipede, legacyObsSpot{1004, 0, 10, 10, 4}},
		{"MT1009 UM982", rtcm1009Um982, legacyObsSpot{1009, 0, 6, 6, 13}},
		{"MT1010 UM982", rtcm1010Um982, legacyObsSpot{1010, 0, 6, 6, 13}},
		{"MT1011 UM982", rtcm1011Um982, legacyObsSpot{1011, 0, 6, 6, 13}},
		{"MT1012 UM982", rtcm1012Um982, legacyObsSpot{1012, 0, 6, 6, 13}},
		{"MT1012 Centipede", rtcm1012Centipede, legacyObsSpot{1012, 0, 10, 10, 2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := ParseMsg(decodePkt(t, tc.hex))
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			got := spotCheck(msg)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestRoundTripLegacyObservables(t *testing.T) {
	tests := []struct {
		name string
		hex  string
	}{
		{"MT1001 UM982", rtcm1001Um982},
		{"MT1002 UM982", rtcm1002Um982},
		{"MT1003 UM982", rtcm1003Um982},
		{"MT1004 UM982", rtcm1004Um982},
		{"MT1004 Centipede", rtcm1004Centipede},
		{"MT1009 UM982", rtcm1009Um982},
		{"MT1010 UM982", rtcm1010Um982},
		{"MT1011 UM982", rtcm1011Um982},
		{"MT1012 UM982", rtcm1012Um982},
		{"MT1012 Centipede", rtcm1012Centipede},
		{"MT1013 Centipede", rtcm1013Centipede},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := decodePkt(t, tc.hex)
			msg, err := ParseMsg(pkt)
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			got, err := SerializeMsg(msg)
			if err != nil {
				t.Fatalf("SerializeMsg: %v", err)
			}
			if got != pkt {
				t.Errorf("round-trip mismatch:\n got %X\nwant %X", got, pkt)
			}
		})
	}
}

// TestMT1011SignedL2L1PRDiff locks in the int14 interpretation of DF047.
// The UM982 MT1011 capture carries the 0x2000 "invalid L2" sentinel in
// L2L1PRDiff; decoding as uint14 would give +8192 instead of -8192.
func TestMT1011SignedL2L1PRDiff(t *testing.T) {
	msg, err := ParseMsg(decodePkt(t, rtcm1011Um982))
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	m := msg.(*MT1011)
	sawSentinel := false
	for _, s := range m.Sat {
		if s.L2L1PRDiff == -8192 {
			sawSentinel = true
			break
		}
	}
	if !sawSentinel {
		t.Errorf("no L2L1PRDiff == -8192 sentinel found; Sat = %+v", m.Sat)
	}
}

func TestMT1013(t *testing.T) {
	// Real Centipede fixture parses to Nm == 0; also include a synthetic
	// non-zero Nm case for announce-slice coverage.
	synthetic := &MT1013{
		MsgHdrStationID: MsgHdrStationID{
			MsgHdr:    MsgHdr{MsgNum: 1013},
			StationID: 1234,
		},
		MJD:          61142,
		SecondsOfDay: 54321,
		Nm:           3,
		LeapSeconds:  18,
		Announce: []MT1013Announce{
			{MsgID: 1005, Sync: true, TxInterval: 10},
			{MsgID: 1074, Sync: false, TxInterval: 10},
			{MsgID: 1230, Sync: false, TxInterval: 50},
		},
	}
	syntheticPkt, err := SerializeMsg(synthetic)
	if err != nil {
		t.Fatalf("SerializeMsg synthetic: %v", err)
	}

	tests := []struct {
		name   string
		pkt    string
		expect *MT1013
	}{
		{
			name: "Centipede nm=0",
			pkt:  decodePkt(t, rtcm1013Centipede),
			expect: &MT1013{
				MsgHdrStationID: MsgHdrStationID{
					MsgHdr:    MsgHdr{MsgNum: 1013},
					StationID: 0,
				},
				MJD:          61142,
				SecondsOfDay: 76018,
				Nm:           0,
				LeapSeconds:  18,
				Announce:     []MT1013Announce{},
			},
		},
		{"synthetic nm=3", syntheticPkt, synthetic},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := ParseMsg(tc.pkt)
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			if !reflect.DeepEqual(msg, tc.expect) {
				t.Errorf("parsed:\n got  %+v\n want %+v", msg, tc.expect)
			}
			got, err := SerializeMsg(msg)
			if err != nil {
				t.Fatalf("SerializeMsg: %v", err)
			}
			if got != tc.pkt {
				t.Errorf("round-trip mismatch:\n got %X\nwant %X", got, tc.pkt)
			}
		})
	}
}

// TestRoundTripEmptyNsat covers the Nsat == 0 edge case neither real
// capture exercises.  Codec self-consistency only — no independent
// encoder.
func TestRoundTripEmptyNsat(t *testing.T) {
	hdr := LegacyGPSHeader{
		MsgHdrStationID: MsgHdrStationID{
			MsgHdr:    MsgHdr{MsgNum: 1001},
			StationID: 42,
		},
		Epoch: 12345,
	}
	src := &MT1001{LegacyGPSHeader: hdr, Sat: []MT1001Sat{}}
	pkt, err := SerializeMsg(src)
	if err != nil {
		t.Fatalf("SerializeMsg: %v", err)
	}
	msg, err := ParseMsg(pkt)
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	if !reflect.DeepEqual(msg, src) {
		t.Errorf("got  %+v\nwant %+v", msg, src)
	}
}

// Real MT1008 and MT1033 captures from the Centipede Ntrip network.
const (
	rtcm1008Centipede = "d3001a3f00001454524d35353937312e303020202020204e4f4e45000069e22f"
	rtcm1033Centipede = "d300364090001454524d35353937312e303020202020204e4f4e4500000a4c45494341204752323508342e33312e31303107313833303333378c14a0"
)

func TestParseDescriptorMsgs(t *testing.T) {
	// Synthetic MT1007: TRIMBLE ZEPHYR antenna, no serial.
	synthetic1007 := &MT1007{
		MsgHdrStationID: MsgHdrStationID{
			MsgHdr:    MsgHdr{MsgNum: 1007},
			StationID: 42,
		},
		DescriptorCounterN: 14,
		AntennaDescriptor:  ASCIIString("TRIMBLE ZEPHYR"),
		AntennaSetupID:     1,
	}
	synthetic1007Pkt, err := SerializeMsg(synthetic1007)
	if err != nil {
		t.Fatalf("serialize synthetic MT1007: %v", err)
	}

	tests := []struct {
		name   string
		pkt    string
		expect Msg
	}{
		{
			name: "MT1007 synthetic",
			pkt:  synthetic1007Pkt,
			expect: synthetic1007,
		},
		{
			name: "MT1008 Centipede",
			pkt:  decodePkt(t, rtcm1008Centipede),
			expect: &MT1008{
				MT1007: MT1007{
					MsgHdrStationID: MsgHdrStationID{
						MsgHdr:    MsgHdr{MsgNum: 1008},
						StationID: 0,
					},
					DescriptorCounterN: 20,
					AntennaDescriptor:  ASCIIString("TRM55971.00     NONE"),
					AntennaSetupID:     0,
				},
				SerialNumberCounterM: 0,
				AntennaSerialNumber:  ASCIIString{},
			},
		},
		{
			name: "MT1033 Centipede",
			pkt:  decodePkt(t, rtcm1033Centipede),
			expect: &MT1033{
				MT1008: MT1008{
					MT1007: MT1007{
						MsgHdrStationID: MsgHdrStationID{
							MsgHdr:    MsgHdr{MsgNum: 1033},
							StationID: 0,
						},
						DescriptorCounterN: 20,
						AntennaDescriptor:  ASCIIString("TRM55971.00     NONE"),
						AntennaSetupID:     0,
					},
					SerialNumberCounterM: 0,
					AntennaSerialNumber:  ASCIIString{},
				},
				ReceiverTypeCounterI:    10,
				ReceiverTypeDescriptor:  ASCIIString("LEICA GR25"),
				FirmwareCounterJ:        8,
				ReceiverFirmwareVersion: ASCIIString("4.31.101"),
				ReceiverSerialCounterK:  7,
				ReceiverSerialNumber:    ASCIIString("1830337"),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseMsg(tc.pkt)
			if err != nil {
				t.Fatalf("ParseMsg: %v", err)
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("parsed:\n got  %+v\n want %+v", got, tc.expect)
			}
			round, err := SerializeMsg(got)
			if err != nil {
				t.Fatalf("SerializeMsg: %v", err)
			}
			if round != tc.pkt {
				t.Errorf("round-trip mismatch:\n got %X\nwant %X", round, tc.pkt)
			}
		})
	}
}

// TestReferenceStationIDDescriptorMsgs regressions the bug where
// ReferenceStationID returned false for 1007/1008/1033 after the switch
// to the interface-based lookup.  The byte-level fast path must still
// work without ParseMsg.
func TestReferenceStationIDDescriptorMsgs(t *testing.T) {
	tests := []struct {
		name string
		pkt  string
		want uint16
	}{
		{"MT1008 Centipede", decodePkt(t, rtcm1008Centipede), 0},
		{"MT1033 Centipede", decodePkt(t, rtcm1033Centipede), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ReferenceStationID([]byte(tc.pkt))
			if !ok {
				t.Fatal("ReferenceStationID returned ok=false")
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSatellitesSignals(t *testing.T) {
	h := MSMHeader{}
	// Set bits 0 (sat 1), 5 (sat 6), 63 (sat 64)
	h.SatMask = 1<<63 | 1<<58 | 1<<0
	sats := h.Satellites()
	want := []uint8{1, 6, 64}
	if len(sats) != len(want) {
		t.Fatalf("Satellites() len = %d, want %d", len(sats), len(want))
	}
	for i, w := range want {
		if sats[i] != w {
			t.Errorf("Satellites()[%d] = %d, want %d", i, sats[i], w)
		}
	}
	// Signal mask: bits 0 (sig 1), 31 (sig 32)
	h.SigMask = 1<<31 | 1<<0
	sigs := h.Signals()
	wantSigs := []uint8{1, 32}
	if len(sigs) != len(wantSigs) {
		t.Fatalf("Signals() len = %d, want %d", len(sigs), len(wantSigs))
	}
	for i, w := range wantSigs {
		if sigs[i] != w {
			t.Errorf("Signals()[%d] = %d, want %d", i, sigs[i], w)
		}
	}
}
