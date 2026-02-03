package asbin

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

// testCase defines a table-driven test case for message parsing.
type testCase struct {
	name    string // test name
	packet  string // hex-encoded packet (spaces allowed)
	wantID  MsgID  // expected message ID
	wantMsg Msg    // expected decoded message (compared with reflect.DeepEqual)
}

// runTest parses the packet and verifies the message ID and decoded payload.
func runTest(t *testing.T, tc testCase) {
	t.Helper()
	packetHex := strings.ReplaceAll(tc.packet, " ", "")
	packetBytes, err := hex.DecodeString(packetHex)
	if err != nil {
		t.Fatalf("invalid hex in test case: %v", err)
	}
	msg, err := ParseMsg(string(packetBytes))
	if err != nil {
		t.Fatalf("ParseMsg failed: %v", err)
	}
	if msg.ID() != tc.wantID {
		t.Errorf("ID = %v, want %v", msg.ID(), tc.wantID)
	}
	if !reflect.DeepEqual(msg, tc.wantMsg) {
		t.Errorf("message mismatch:\ngot:  %+v\nwant: %+v", msg, tc.wantMsg)
	}
}

// runTests runs a slice of test cases as subtests.
func runTests(t *testing.T, tests []testCase) {
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runTest(t, tc)
		})
	}
}

// hexToBytes converts a hex string (spaces allowed) to bytes.
func hexToBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatalf("invalid hex: %v", err)
	}
	return b
}

func TestMsgIDString(t *testing.T) {
	tests := []struct {
		mid  MsgID
		want string
	}{
		// Messages with decoders
		{NavTimeID, "NAV-TIME"},
		{NavClockID, "NAV-CLOCK"},
		{NavSvinID, "NAV-SVIN"},
		{AckNakID, "ACK-NAK"},
		{AckAckID, "ACK-ACK"},
		{CfgPrtID, "CFG-PRT"},
		{CfgMsgID, "CFG-MSG"},
		{CfgPpsID, "CFG-PPS"},
		{CfgNavSatID, "CFG-NAVSAT"},
		{CfgSimpleRstID, "CFG-SIMPLERST"},
		{MonVerID, "MON-VER"},
		// Messages without decoders (from other.go)
		{NavPosEcefID, "NAV-POSECEF"},
		{NavPosLlhID, "NAV-POSLLH"},
		{NavDopID, "NAV-DOP"},
		{NavVelEcefID, "NAV-VELECEF"},
		{NavVelNedID, "NAV-VELNED"},
		{NavTimeUtcID, "NAV-TIMEUTC"},
		{NavPvErrID, "NAV-PVERR"},
		{NavSvInfoID, "NAV-SVINFO"},
		{NavSvStateID, "NAV-SVSTATE"},
		{NavAutoID, "NAV-AUTO"},
		{NavNavPvtID, "NAV-NAVPVT"},
		{CfgDopID, "CFG-DOP"},
		{CfgElevID, "CFG-ELEV"},
		{CfgHeightID, "CFG-HEIGHT"},
		{CfgSbasID, "CFG-SBAS"},
		{CfgSpdHoldID, "CFG-SPDHOLD"},
		{CfgEphSaveID, "CFG-EPHSAVE"},
		{CfgNumSvID, "CFG-NUMSV"},
		{CfgFixedLlaID, "CFG-FIXEDLLA"},
		{CfgAntiJamID, "CFG-ANTIJAM"},
		{CfgGeoFenceID, "CFG-GEOFENCE"},
		{CfgCarrSmthID, "CFG-CARRSMOOTH"},
		{CfgBdGeoID, "CFG-BDGEO"},
		{CfgFwUpID, "CFG-FWUP"},
		{CfgPvtLogID, "CFG-PVTLOG"},
		{CfgNmeaVerID, "CFG-NMEAVER"},
		{CfgPwrCtlID, "CFG-PWRCTL"},
		{CfgSleepID, "CFG-SLEEP"},
		{MonInfoID, "MON-INFO"},
		{MonTrkChanID, "MON-TRKCHAN"},
		{MonRcvClkID, "MON-RCVCLK"},
		{MonCwiID, "MON-CWI"},
		{AidIniID, "AID-INI"},
		{AidTimeID, "AID-TIME"},
		{AidPosID, "AID-POS"},
		{AidPalmGlnID, "AID-PALMGLN"},
		{AidPalmGpsID, "AID-PALMGPS"},
		{AidPalmBdID, "AID-PALMBD"},
		{AidPalmGalID, "AID-PALMGAL"},
		{AidPalmQzssID, "AID-PALMQZSS"},
		{AidEphGpsID, "AID-EPHGPS"},
		{AidEphBdID, "AID-EPHBD"},
		// Unknown message should use hex format
		{MakeMsgID(0x99, 0x99), "0x99-0x99"},
	}
	for _, tc := range tests {
		got := tc.mid.String()
		if got != tc.want {
			t.Errorf("MsgID(%#04x).String() = %q, want %q", uint16(tc.mid), got, tc.want)
		}
	}
}
