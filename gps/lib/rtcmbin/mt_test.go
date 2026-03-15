package rtcmbin

import (
	"encoding/hex"
	"testing"
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
	h := MSMHeader{MsgHdr: MsgHdr{MsgNum: 1074}}
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
		h := MSMHeader{MsgHdr: MsgHdr{MsgNum: tt.mt}}
		if got := h.GNSS(); got != tt.want {
			t.Errorf("GNSS(%d) = %q, want %q", tt.mt, got, tt.want)
		}
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

// Real MSM4 packets from the plan.
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

func TestParseMSM4(t *testing.T) {
	for _, tt := range msmPackets {
		t.Run(tt.name, func(t *testing.T) {
			pkt := decodePkt(t, tt.hex)
			result, err := ParseMsg(pkt)
			if err != nil {
				t.Fatal(err)
			}
			msg, ok := result.(*MSM)
			if !ok {
				t.Fatalf("ParseMsg returned %T, want *MSM", result)
			}
			if msg.MsgNum != tt.mt {
				t.Errorf("MsgNum = %d, want %d", msg.MsgNum, tt.mt)
			}
			if msg.GNSS() != tt.gnss {
				t.Errorf("GNSS() = %q, want %q", msg.GNSS(), tt.gnss)
			}
			if msg.MSMLevel() != 4 {
				t.Errorf("MSMLevel() = %d, want 4", msg.MSMLevel())
			}
			nsat := msg.Nsat()
			nsig := msg.Nsig()
			if nsat == 0 {
				t.Error("Nsat = 0")
			}
			if nsig == 0 {
				t.Error("Nsig = 0")
			}
			t.Logf("%s: station=%d nsat=%d nsig=%d sats=%v sigs=%v",
				tt.gnss, msg.StationID, nsat, nsig, msg.Satellites(), msg.Signals())
			// MSM4 satellite data: RangeInt (nsat), RangeMod (nsat)
			if len(msg.Sat.RangeInt) != nsat {
				t.Errorf("RangeInt len = %d, want %d", len(msg.Sat.RangeInt), nsat)
			}
			if len(msg.Sat.RangeMod) != nsat {
				t.Errorf("RangeMod len = %d, want %d", len(msg.Sat.RangeMod), nsat)
			}
			// MSM4: ExtInfo and PhaseRate absent
			if len(msg.Sat.ExtInfo) != 0 {
				t.Errorf("ExtInfo len = %d, want 0", len(msg.Sat.ExtInfo))
			}
			if len(msg.Sat.PhaseRate) != 0 {
				t.Errorf("PhaseRate len = %d, want 0", len(msg.Sat.PhaseRate))
			}
			// MSM4 signal data: Pseudorange, PhaseRange, LockTime, HalfCycle, CNR
			ncell := len(msg.Sig.Pseudorange)
			if ncell == 0 {
				t.Error("Pseudorange len = 0")
			}
			if len(msg.Sig.PhaseRange) != ncell {
				t.Errorf("PhaseRange len = %d, want %d", len(msg.Sig.PhaseRange), ncell)
			}
			if len(msg.Sig.CNR) != ncell {
				t.Errorf("CNR len = %d, want %d", len(msg.Sig.CNR), ncell)
			}
			// MSM4: no signal PhaseRate
			if len(msg.Sig.PhaseRate) != 0 {
				t.Errorf("Sig.PhaseRate len = %d, want 0", len(msg.Sig.PhaseRate))
			}
			// Cross-check station ID with ReferenceStationID extraction
			wantID, idOK := ReferenceStationID([]byte(pkt))
			if !idOK {
				t.Fatal("ReferenceStationID returned false")
			}
			if msg.StationID != wantID {
				t.Errorf("StationID = %d, want %d", msg.StationID, wantID)
			}
		})
	}
}

func TestParseMSM4StationID(t *testing.T) {
	// Verify all MSM4 packets have the same station ID (from same receiver).
	pkt0 := decodePkt(t, msmPackets[0].hex)
	msg0, err := ParseMsg(pkt0)
	if err != nil {
		t.Fatal(err)
	}
	wantID := msg0.(*MSM).StationID
	for _, tt := range msmPackets[1:] {
		pkt := decodePkt(t, tt.hex)
		result, err := ParseMsg(pkt)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		msg := result.(*MSM)
		if msg.StationID != wantID {
			t.Errorf("%s: StationID = %d, want %d", tt.name, msg.StationID, wantID)
		}
	}
}
