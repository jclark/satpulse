package bin

import (
	"strings"
	"testing"

	"golang.org/x/exp/slices"
)

func TestNavTimeGPS(t *testing.T) {
	testMsgType(t, NavTimeGPS{
		ITOW:  0x12345678,
		FTOW:  0,
		Week:  6523,
		LeapS: 37,
		Valid: 0,
		TAcc:  173,
	})
}

func TestCfgCfg(t *testing.T) {
	m := CfgCfg{
		CfgCfgFixed{
			ClearMask: CfgCfgIOPort,
			SaveMask:  CfgCfgRXMConf,
			LoadMask:  CfgCfgNavConf,
		},
		[]CfgCfgDeviceMask{CfgCfgDevFlash},
	}
	p2 := testMsgType1(t, m)
	if !EqualCfgCfg(&m, p2.(*CfgCfg)) {
		t.Fatalf("msg cfg-cfg not roundtripped %v => %v", &m, p2)
	}
	m = CfgCfg{
		CfgCfgFixed{
			ClearMask: CfgCfgIOPort,
			SaveMask:  CfgCfgRXMConf,
			LoadMask:  CfgCfgNavConf,
		},
		[]CfgCfgDeviceMask{},
	}
	p2 = testMsgType1(t, m)
	if !EqualCfgCfg(&m, p2.(*CfgCfg)) {
		t.Fatalf("msg cfg-cfg not roundtripped %v => %v", &m, p2)
	}
}

func EqualCfgCfg(p1, p2 *CfgCfg) bool {
	return p1.CfgCfgFixed == p2.CfgCfgFixed && slices.Equal(p1.DeviceMask, p2.DeviceMask)
}

func TestMonVer(t *testing.T) {
	m := MonVer{
		MonVerFixed{[30]byte{1, 2, 3}, [10]byte{4, 5}},
		[][30]byte{{7, 8}, {9}},
	}
	p2 := testMsgType1(t, m)
	if !EqualMonVer(&m, p2.(*MonVer)) {
		t.Fatalf("msg mon-ver not roundtripped %v => %v", &m, p2)
	}
}

func EqualMonVer(p1, p2 *MonVer) bool {
	return p1.MonVerFixed == p2.MonVerFixed && slices.Equal(p1.Extension, p2.Extension)
}

func TestCfgValget(t *testing.T) {
	m := CfgValget{
		CfgValgetFixed{
			Version:  CfgValgetVersionResponse,
			Layer:    CfgValgetLayerRAM,
			Position: 0,
		},
		[]byte{1, 2, 3, 4, 5},
	}
	p2 := testMsgType1(t, m)
	if !EqualCfgValget(&m, p2.(*CfgValget)) {
		t.Fatalf("msg cfg-valget not roundtripped %v => %v", &m, p2)
	}
}

func EqualCfgValget(p1, p2 *CfgValget) bool {
	return p1.CfgValgetFixed == p2.CfgValgetFixed && slices.Equal(p1.CfgData, p2.CfgData)
}

func TestInf(t *testing.T) {
	bytes := ([]byte)("hello")
	m := InfDebug{InfText(bytes)}
	p2 := testMsgType1(t, m)
	if !slices.Equal(bytes, ([]byte)(p2.(*InfDebug).InfText)) {
		t.Fatalf("msg inf not roundtripped %v => %v", &m, p2)
	}
}

func testMsgType[M comparable, PM interface {
	ID() MsgID
	*M
}](t *testing.T, m M) {
	p2 := testMsgType1[M, PM](t, m)
	m2 := *p2.(PM)
	if m2 != m {
		p := PM(&m)
		mid := p.ID()
		t.Fatalf("msg %v not roundtripped %v => %v", mid, &p, &p2)
	}
}

func testMsgType1[M any, PM interface {
	ID() MsgID
	*M
}](t *testing.T, m M) Msg {
	p := PM(&m)
	mid := p.ID()
	b, err := Serialize(p)
	if err != nil {
		t.Fatalf("serialize err for %v: %v", mid, err)
	}
	p2, err := ParseMsg(string(b))
	if err != nil {
		t.Fatalf("parse error for %v: %v", mid, err)
	}
	if p2.ID() != p.ID() {
		t.Fatalf("msgid not roundtripped %v => %v", mid, p2.ID())
	}
	return p2
}

func TestTrailingBytes(t *testing.T) {
	a := AckAck{}
	b, _ := packMsg(a.ID(), []byte{clsCfg, 0x01,
		// extra byte
		0x42})
	s := string(b)
	_, err := ParseMsg(s)
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("failed to give an error for trailing bytes")
	}
}

func TestPoll(t *testing.T) {
	buf := Poll(MonVerID)
	if len(buf) != packetMinLength {
		t.Fatalf("unexpected output length %d (expected %d)", len(buf), packetMinLength)
	}
	if buf[2] != clsMon {
		t.Fatalf("invalid cls byte")
	}
}

func TestMonGnssV0(t *testing.T) {
	m := MonGnss{
		Version: 0,
		MonGnssV0: MonGnssV0{
			Supported:    MonGnssGPS | MonGnssGalileo,
			DefaultGnss:  MonGnssGPS,
			Enabled:      MonGnssGPS | MonGnssBeidou,
			Simultaneous: 4,
		},
	}
	p2 := testMsgType1(t, m)
	if !EqualMonGnss(&m, p2.(*MonGnss)) {
		t.Fatalf("msg mon-gnss v0 not roundtripped %v => %v", &m, p2)
	}
}

func TestMonGnssV1(t *testing.T) {
	m := MonGnss{
		Version: 1,
		Unknown: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
	}
	p2 := testMsgType1(t, m)
	if !EqualMonGnss(&m, p2.(*MonGnss)) {
		t.Fatalf("msg mon-gnss v1 not roundtripped %v => %v", &m, p2)
	}
}

func EqualMonGnss(p1, p2 *MonGnss) bool {
	if p1.Version != p2.Version {
		return false
	}
	if p1.Version == 0 {
		return p1.MonGnssV0 == p2.MonGnssV0
	}
	return slices.Equal(p1.Unknown, p2.Unknown)
}

func TestMgaIniTimeUTC(t *testing.T) {
	// Test constants
	const (
		msgType     = MgaIniTypeTimeUTC // TIME_UTC
		msgVersion  = 0x00
		refValue    = 0x05 // bits 3:0=source=1, bit 4=fall=0, bit 5=last=1
		leapSecs    = 18
		year        = 2024
		month       = 12
		day         = 15
		hour        = 10
		minute      = 30
		second      = 45
		bitfield0   = 0x01 // trustedSource=1
		ns          = 1000000000
		tAccS       = 5
		tAccNs      = 1000000000
	)

	// Manually construct the expected byte sequence for UBX-MGA-INI-TIME-UTC
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x13, 0x40, // class=MGA(0x13), id=INI(0x40)
		0x18, 0x00, // length = 24 bytes
		// Payload starts here:
		byte(msgType),             // type = 0x10 (TIME_UTC)
		msgVersion,          // version = 0x00
		refValue,            // ref = 0x05
		leapSecs,            // leapSecs = 18
		year & 0xFF, year >> 8, // year = 2024 (little endian)
		month,               // month = 12
		day,                 // day = 15
		hour,                // hour = 10
		minute,              // minute = 30
		second,              // second = 45
		bitfield0,           // bitfield0 = 0x01
		byte(ns & 0xFF), byte((ns >> 8) & 0xFF), byte((ns >> 16) & 0xFF), byte((ns >> 24) & 0xFF), // ns (little endian)
		byte(tAccS & 0xFF), byte((tAccS >> 8) & 0xFF), // tAccS (little endian)
		0x00, 0x00, // reserved
		byte(tAccNs & 0xFF), byte((tAccNs >> 8) & 0xFF), byte((tAccNs >> 16) & 0xFF), byte((tAccNs >> 24) & 0xFF), // tAccNs (little endian)
	}
	
	// Calculate and append checksum
	ckA, ckB := Checksum(packet[2:]) // Skip sync bytes for checksum
	packet = append(packet, ckA, ckB)

	// Parse the message
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse MGA-INI-TIME-UTC: %v", err)
	}

	// Verify it's the right type
	mgaIni, ok := msg.(*MgaIni)
	if !ok {
		t.Fatalf("expected *MgaIni, got %T", msg)
	}

	// Check fixed part
	if mgaIni.Type != msgType {
		t.Errorf("expected Type=0x%02x, got 0x%02x", msgType, mgaIni.Type)
	}
	if mgaIni.Version != msgVersion {
		t.Errorf("expected Version=0x%02x, got 0x%02x", msgVersion, mgaIni.Version)
	}

	// Check varying part is TimeUTC
	timeUTC, ok := mgaIni.VaryingPart().(*MgaIniTimeUTC)
	if !ok {
		t.Fatalf("expected *MgaIniTimeUTC, got %T", mgaIni.VaryingPart())
	}

	// Verify all fields using constants
	if timeUTC.Ref != refValue {
		t.Errorf("expected Ref=0x%02x, got 0x%02x", refValue, timeUTC.Ref)
	}
	if timeUTC.LeapSecs != leapSecs {
		t.Errorf("expected LeapSecs=%d, got %d", leapSecs, timeUTC.LeapSecs)
	}
	if timeUTC.Year != year {
		t.Errorf("expected Year=%d, got %d", year, timeUTC.Year)
	}
	if timeUTC.Month != month {
		t.Errorf("expected Month=%d, got %d", month, timeUTC.Month)
	}
	if timeUTC.Day != day {
		t.Errorf("expected Day=%d, got %d", day, timeUTC.Day)
	}
	if timeUTC.Hour != hour {
		t.Errorf("expected Hour=%d, got %d", hour, timeUTC.Hour)
	}
	if timeUTC.Minute != minute {
		t.Errorf("expected Minute=%d, got %d", minute, timeUTC.Minute)
	}
	if timeUTC.Second != second {
		t.Errorf("expected Second=%d, got %d", second, timeUTC.Second)
	}
	if timeUTC.Bitfield0 != bitfield0 {
		t.Errorf("expected Bitfield0=0x%02x, got 0x%02x", bitfield0, timeUTC.Bitfield0)
	}
	if timeUTC.Ns != ns {
		t.Errorf("expected Ns=%d, got %d", ns, timeUTC.Ns)
	}
	if timeUTC.TAccS != tAccS {
		t.Errorf("expected TAccS=%d, got %d", tAccS, timeUTC.TAccS)
	}
	if timeUTC.TAccNs != tAccNs {
		t.Errorf("expected TAccNs=%d, got %d", tAccNs, timeUTC.TAccNs)
	}

	// Test serialization by creating a new message with the same data
	newMsg := &MgaIni{
		MgaIniFixed: MgaIniFixed{
			Type:    msgType,
			Version: msgVersion,
		},
		payload: &MgaIniTimeUTC{
			Ref:       refValue,
			LeapSecs:  leapSecs,
			Year:      year,
			Month:     month,
			Day:       day,
			Hour:      hour,
			Minute:    minute,
			Second:    second,
			Bitfield0: bitfield0,
			Ns:        ns,
			TAccS:     tAccS,
			TAccNs:    tAccNs,
		},
	}

	serialized, err := Serialize(newMsg)
	if err != nil {
		t.Fatalf("failed to serialize MGA-INI-TIME-UTC: %v", err)
	}

	if !slices.Equal(serialized, packet) {
		t.Errorf("serialization mismatch:\nexpected: %x\ngot:      %x", packet, serialized)
	}
}

func TestMgaIniUnknown(t *testing.T) {
	// Test unknown message type with version 0 (should succeed as unknown after the fix)
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x13, 0x40, // class=MGA(0x13), id=INI(0x40)
		0x0A, 0x00, // length = 10 bytes
		// Payload starts here:
		0x30,       // type = 0x30 (unknown type - not implemented)
		0x00,       // version = 0x00
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // 8 bytes of data
	}
	
	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)

	// This should succeed as unknown (after the fix)
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse unknown MGA-INI with version 0: %v", err)
	}

	mgaIni, ok := msg.(*MgaIni)
	if !ok {
		t.Fatalf("expected *MgaIni, got %T", msg)
	}

	if mgaIni.Type != 0x30 {
		t.Errorf("expected Type=0x30, got 0x%02x", mgaIni.Type)
	}
	if mgaIni.Version != 0x00 {
		t.Errorf("expected Version=0x00, got 0x%02x", mgaIni.Version)
	}

	unknown, ok := mgaIni.VaryingPart().(*MgaIniUnknown)
	if !ok {
		t.Fatalf("expected *MgaIniUnknown, got %T", mgaIni.VaryingPart())
	}

	expectedUnknown := MgaIniUnknown{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if !slices.Equal(*unknown, expectedUnknown) {
		t.Errorf("expected unknown payload=%x, got %x", expectedUnknown, *unknown)
	}

	// Test unknown message type with version 1 (should also succeed as unknown)
	packet[7] = 0x01 // Change version to 1
	ckA, ckB = Checksum(packet[2:len(packet)-2])
	packet[len(packet)-2] = ckA
	packet[len(packet)-1] = ckB

	msg, err = ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse unknown MGA-INI with version 1: %v", err)
	}

	mgaIni, ok = msg.(*MgaIni)
	if !ok {
		t.Fatalf("expected *MgaIni, got %T", msg)
	}

	if mgaIni.Type != 0x30 {
		t.Errorf("expected Type=0x30, got 0x%02x", mgaIni.Type)
	}
	if mgaIni.Version != 0x01 {
		t.Errorf("expected Version=0x01, got 0x%02x", mgaIni.Version)
	}

	unknown, ok = mgaIni.VaryingPart().(*MgaIniUnknown)
	if !ok {
		t.Fatalf("expected *MgaIniUnknown, got %T", mgaIni.VaryingPart())
	}

	expectedUnknown = MgaIniUnknown{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if !slices.Equal(*unknown, expectedUnknown) {
		t.Errorf("expected unknown payload=%x, got %x", expectedUnknown, *unknown)
	}
}

func TestMgaGalOSNMAPubkey(t *testing.T) {
	// Test constants for UBX-MGA-GAL-OSNMA-PUBKEY
	const (
		msgType     = MgaGalTypeOSNMAPubkey // 0x07
		msgVersion  = 0x00
		pubKeyID    = 0x05 // Public Key ID (bits 3:0)
		pubKeyType  = 0x01 // ECDSA P-256 (bits 7:4)
	)

	// Construct bitfield0 = pubKeyType << 4 | pubKeyID
	bitfield0 := MgaGalOSNMAPubkeyBitfield0((pubKeyType << 4) | pubKeyID)

	// Test public key point (67 bytes of test data)
	var pubKeyPoint [67]byte
	for i := range pubKeyPoint {
		pubKeyPoint[i] = byte(i + 1) // Fill with 1, 2, 3, ... 67
	}

	// Manually construct the expected byte sequence for UBX-MGA-GAL-OSNMA-PUBKEY
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x13, 0x02, // class=MGA(0x13), id=GAL(0x02)
		0x48, 0x00, // length = 72 bytes
		// Payload starts here:
		byte(msgType),   // type = 0x07 (OSNMA_PUBKEY)
		msgVersion,      // version = 0x00
		byte(bitfield0), // bitfield0 = (pubKeyType << 4) | pubKeyID
		0x00,            // reserved
	}
	
	// Append the 67-byte public key point
	packet = append(packet, pubKeyPoint[:]...)
	// Append final reserved byte
	packet = append(packet, 0x00)

	// Calculate and append checksum
	ckA, ckB := Checksum(packet[2:]) // Skip sync bytes for checksum
	packet = append(packet, ckA, ckB)

	// Parse the message
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse MGA-GAL-OSNMA-PUBKEY: %v", err)
	}

	// Verify it's the right type
	mgaGal, ok := msg.(*MgaGal)
	if !ok {
		t.Fatalf("expected *MgaGal, got %T", msg)
	}

	// Check fixed part
	if mgaGal.Type != msgType {
		t.Errorf("expected Type=0x%02x, got 0x%02x", msgType, mgaGal.Type)
	}
	if mgaGal.Version != msgVersion {
		t.Errorf("expected Version=0x%02x, got 0x%02x", msgVersion, mgaGal.Version)
	}

	// Check varying part is OSNMAPubkey
	pubkey, ok := mgaGal.VaryingPart().(*MgaGalOSNMAPubkey)
	if !ok {
		t.Fatalf("expected *MgaGalOSNMAPubkey, got %T", mgaGal.VaryingPart())
	}

	// Verify bitfield and extracted values
	if pubkey.Bitfield0 != bitfield0 {
		t.Errorf("expected Bitfield0=0x%02x, got 0x%02x", bitfield0, pubkey.Bitfield0)
	}
	if pubkey.Bitfield0.PubKeyID() != pubKeyID {
		t.Errorf("expected PubKeyID=%d, got %d", pubKeyID, pubkey.Bitfield0.PubKeyID())
	}
	if pubkey.Bitfield0.PubKeyType() != pubKeyType {
		t.Errorf("expected PubKeyType=%d, got %d", pubKeyType, pubkey.Bitfield0.PubKeyType())
	}
	if pubkey.PubKeyPoint != pubKeyPoint {
		t.Errorf("expected PubKeyPoint=%x, got %x", pubKeyPoint, pubkey.PubKeyPoint)
	}

	// Test serialization roundtrip
	newMsg := &MgaGal{
		MgaGalFixed: MgaGalFixed{
			Type:    msgType,
			Version: msgVersion,
		},
		payload: &MgaGalOSNMAPubkey{
			Bitfield0:   bitfield0,
			PubKeyPoint: pubKeyPoint,
		},
	}

	serialized, err := Serialize(newMsg)
	if err != nil {
		t.Fatalf("failed to serialize MGA-GAL-OSNMA-PUBKEY: %v", err)
	}

	if !slices.Equal(serialized, packet) {
		t.Errorf("serialization mismatch:\nexpected: %x\ngot:      %x", packet, serialized)
	}
}

func TestMgaGalOSNMAMerkle(t *testing.T) {
	// Test constants for UBX-MGA-GAL-OSNMA-MERKLE
	const (
		msgType           = MgaGalTypeOSNMAMerkle // 0x08
		msgVersion        = 0x00
		applicabilityTime = true // bit 0 = 1 (future key)
	)

	// Construct bitfield0
	var bitfield0 MgaGalOSNMAMerkleBitfield0
	if applicabilityTime {
		bitfield0 |= MgaGalOSNMAMerkleApplicabilityTime
	}

	// Test Merkle tree node (32 bytes of test data)
	var treeNode [32]byte
	for i := range treeNode {
		treeNode[i] = byte(0xFF - i) // Fill with 255, 254, 253, ... 224
	}

	// Manually construct the expected byte sequence for UBX-MGA-GAL-OSNMA-MERKLE
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x13, 0x02, // class=MGA(0x13), id=GAL(0x02)
		0x24, 0x00, // length = 36 bytes
		// Payload starts here:
		byte(msgType),   // type = 0x08 (OSNMA_MERKLE)
		msgVersion,      // version = 0x00
		byte(bitfield0), // bitfield0 = applicabilityTime bit
		0x00,            // reserved
	}
	
	// Append the 32-byte tree node
	packet = append(packet, treeNode[:]...)

	// Calculate and append checksum
	ckA, ckB := Checksum(packet[2:]) // Skip sync bytes for checksum
	packet = append(packet, ckA, ckB)

	// Parse the message
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse MGA-GAL-OSNMA-MERKLE: %v", err)
	}

	// Verify it's the right type
	mgaGal, ok := msg.(*MgaGal)
	if !ok {
		t.Fatalf("expected *MgaGal, got %T", msg)
	}

	// Check fixed part
	if mgaGal.Type != msgType {
		t.Errorf("expected Type=0x%02x, got 0x%02x", msgType, mgaGal.Type)
	}
	if mgaGal.Version != msgVersion {
		t.Errorf("expected Version=0x%02x, got 0x%02x", msgVersion, mgaGal.Version)
	}

	// Check varying part is OSNMAMerkle
	merkle, ok := mgaGal.VaryingPart().(*MgaGalOSNMAMerkle)
	if !ok {
		t.Fatalf("expected *MgaGalOSNMAMerkle, got %T", mgaGal.VaryingPart())
	}

	// Verify bitfield and extracted values
	if merkle.Bitfield0 != bitfield0 {
		t.Errorf("expected Bitfield0=0x%02x, got 0x%02x", bitfield0, merkle.Bitfield0)
	}
	if merkle.Bitfield0.ApplicabilityTime() != applicabilityTime {
		t.Errorf("expected ApplicabilityTime=%t, got %t", applicabilityTime, merkle.Bitfield0.ApplicabilityTime())
	}
	if merkle.TreeNode != treeNode {
		t.Errorf("expected TreeNode=%x, got %x", treeNode, merkle.TreeNode)
	}

	// Test serialization roundtrip
	newMsg := &MgaGal{
		MgaGalFixed: MgaGalFixed{
			Type:    msgType,
			Version: msgVersion,
		},
		payload: &MgaGalOSNMAMerkle{
			Bitfield0: bitfield0,
			TreeNode:  treeNode,
		},
	}

	serialized, err := Serialize(newMsg)
	if err != nil {
		t.Fatalf("failed to serialize MGA-GAL-OSNMA-MERKLE: %v", err)
	}

	if !slices.Equal(serialized, packet) {
		t.Errorf("serialization mismatch:\nexpected: %x\ngot:      %x", packet, serialized)
	}
}

func TestMgaGalUnknown(t *testing.T) {
	// Test unknown message type with version 0 (should now succeed as unknown)
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x13, 0x02, // class=MGA(0x13), id=GAL(0x02)
		0x0A, 0x00, // length = 10 bytes
		// Payload starts here:
		0x01,       // type = 0x01 (EPH - not implemented)
		0x00,       // version = 0x00
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // 8 bytes of data
	}
	
	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)

	// This should now succeed as unknown (after the fix)
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse unknown MGA-GAL with version 0: %v", err)
	}

	mgaGal, ok := msg.(*MgaGal)
	if !ok {
		t.Fatalf("expected *MgaGal, got %T", msg)
	}

	if mgaGal.Type != 0x01 {
		t.Errorf("expected Type=0x01, got 0x%02x", mgaGal.Type)
	}
	if mgaGal.Version != 0x00 {
		t.Errorf("expected Version=0x00, got 0x%02x", mgaGal.Version)
	}

	unknown, ok := mgaGal.VaryingPart().(*MgaGalUnknown)
	if !ok {
		t.Fatalf("expected *MgaGalUnknown, got %T", mgaGal.VaryingPart())
	}

	expectedUnknown := MgaGalUnknown{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if !slices.Equal(*unknown, expectedUnknown) {
		t.Errorf("expected unknown payload=%x, got %x", expectedUnknown, *unknown)
	}

	// Test unknown message type with version 1 (should also succeed as unknown)
	packet[7] = 0x01 // Change version to 1
	ckA, ckB = Checksum(packet[2:len(packet)-2])
	packet[len(packet)-2] = ckA
	packet[len(packet)-1] = ckB

	msg, err = ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse unknown MGA-GAL with version 1: %v", err)
	}

	mgaGal, ok = msg.(*MgaGal)
	if !ok {
		t.Fatalf("expected *MgaGal, got %T", msg)
	}

	if mgaGal.Type != 0x01 {
		t.Errorf("expected Type=0x01, got 0x%02x", mgaGal.Type)
	}
	if mgaGal.Version != 0x01 {
		t.Errorf("expected Version=0x01, got 0x%02x", mgaGal.Version)
	}

	unknown, ok = mgaGal.VaryingPart().(*MgaGalUnknown)
	if !ok {
		t.Fatalf("expected *MgaGalUnknown, got %T", mgaGal.VaryingPart())
	}

	expectedUnknown = MgaGalUnknown{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	if !slices.Equal(*unknown, expectedUnknown) {
		t.Errorf("expected unknown payload=%x, got %x", expectedUnknown, *unknown)
	}
}
