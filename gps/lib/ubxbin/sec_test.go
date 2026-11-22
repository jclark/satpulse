package ubxbin

import (
	"slices"
	"testing"
)

func TestSecOsnma(t *testing.T) {
	// Test constants for UBX-SEC-OSNMA v3
	const (
		msgVersion = 0x03

		// NMA Header fields
		headerAuthStatus = true
		nmaStatus        = 2 // Operational
		chainInForce     = 1
		cpks             = 3 // Chain revoked

		// OSNMA Monitoring fields
		osnmaEnabled    = true
		numberSVs       = 12
		nmaHeaderUpdate = 1 // Healthy
		noData          = false
		wrongData       = false
		wrongFlxMac     = true
		wrongMaclt      = false

		// Time Sync fields
		timSyncEnabled = true
		timSyncStatus  = 3       // Passed
		timSyncReqDiff = -125000 // ms

		// DSM Authentication fields
		dsmAuthStatus  = 1 // KROOT OK
		hashFunction   = 2
		macFunction    = 1
		pubKeyId       = 5
		macLookupTable = 127
		keySize        = 8
		macSize        = 10
		fromNVS        = true

		// TESLA Key fields
		teslaKeyAuthStatus = 1 // OK
		wnSf               = 2100
		towSf              = 12345
		chainId            = 2

		// General and Timing fields
		authSVsCount        = 2 // Number of AuthSVs to include
		authNumTim          = 15
		timingAuthResult    = 1 // OK
		macAdkdTypeSlow     = true
		pubKeySrc           = 2 // Aided
		merkleRootSrc       = 3 // NVS
		merkleRootVal       = true
		futureMerkleRootSrc = 2 // Aided
		futureMerkleRootVal = false
		pubKeyVal           = true
		futurePubKeyVal     = true
		futurePubKeySrc     = 1 // Satellites
		futurePubKeyId      = 7
	)

	// Construct bitfields
	var nmaHeader SecOsnmaNmaHeader
	if headerAuthStatus {
		nmaHeader |= SecOsnmaHeaderAuthStatus
	}
	nmaHeader |= SecOsnmaNmaHeader(nmaStatus << 1)
	nmaHeader |= SecOsnmaNmaHeader(chainInForce << 3)
	nmaHeader |= SecOsnmaNmaHeader(cpks << 5)

	var osnmaMonitoring SecOsnmaMonitoring
	if osnmaEnabled {
		osnmaMonitoring |= SecOsnmaMonitoringOsnmaEnabled
	}
	osnmaMonitoring |= SecOsnmaMonitoring(numberSVs << 1)
	osnmaMonitoring |= SecOsnmaMonitoring(nmaHeaderUpdate << 6)
	if noData {
		osnmaMonitoring |= SecOsnmaMonitoringNoData
	}
	if wrongData {
		osnmaMonitoring |= SecOsnmaMonitoringWrongData
	}
	if wrongFlxMac {
		osnmaMonitoring |= SecOsnmaMonitoringWrongFlxMac
	}
	if wrongMaclt {
		osnmaMonitoring |= SecOsnmaMonitoringWrongMaclt
	}

	var timSyncReq SecOsnmaTimSyncReq
	if timSyncEnabled {
		timSyncReq |= SecOsnmaTimSyncEnabled
	}
	timSyncReq |= SecOsnmaTimSyncReq(timSyncStatus << 1)

	var dsmAuthentication SecOsnmaDsmAuth
	dsmAuthentication |= SecOsnmaDsmAuth(dsmAuthStatus)
	dsmAuthentication |= SecOsnmaDsmAuth(hashFunction << 6)
	dsmAuthentication |= SecOsnmaDsmAuth(macFunction << 8)
	dsmAuthentication |= SecOsnmaDsmAuth(pubKeyId << 10)
	dsmAuthentication |= SecOsnmaDsmAuth(macLookupTable << 14)
	dsmAuthentication |= SecOsnmaDsmAuth(keySize << 22)
	dsmAuthentication |= SecOsnmaDsmAuth(macSize << 26)
	if fromNVS {
		dsmAuthentication |= SecOsnmaDsmAuthFromNVS
	}

	var teslaKey SecOsnmaTeslaKey
	teslaKey |= SecOsnmaTeslaKey(teslaKeyAuthStatus)
	teslaKey |= SecOsnmaTeslaKey(wnSf << 3)
	teslaKey |= SecOsnmaTeslaKey(towSf << 15)
	teslaKey |= SecOsnmaTeslaKey(chainId << 30)

	var generalAndTiming SecOsnmaGeneral
	generalAndTiming |= SecOsnmaGeneral(authSVsCount)
	generalAndTiming |= SecOsnmaGeneral(authNumTim << 6)
	generalAndTiming |= SecOsnmaGeneral(timingAuthResult << 12)
	if macAdkdTypeSlow {
		generalAndTiming |= SecOsnmaMacAdkdTypeSlow
	}
	generalAndTiming |= SecOsnmaGeneral(pubKeySrc << 15)
	generalAndTiming |= SecOsnmaGeneral(merkleRootSrc << 17)
	if merkleRootVal {
		generalAndTiming |= SecOsnmaGeneralMerkleRootVal
	}
	generalAndTiming |= SecOsnmaGeneral(futureMerkleRootSrc << 20)
	if futureMerkleRootVal {
		generalAndTiming |= SecOsnmaGeneralFutureMerkleRootVal
	}
	if pubKeyVal {
		generalAndTiming |= SecOsnmaGeneralPubKeyVal
	}
	if futurePubKeyVal {
		generalAndTiming |= SecOsnmaGeneralFuturePubKeyVal
	}
	generalAndTiming |= SecOsnmaGeneral(futurePubKeySrc << 25)
	generalAndTiming |= SecOsnmaGeneral(futurePubKeyId << 27)

	// Create AuthSVs test data
	authSVs := []SecOsnmaAuthSV{
		{
			Bitfield1: SecOsnmaAuthSVBitfield(512 | (5 << 10) | (1 << 15)), // IODE=512, AuthNum=5, AuthStatus=fail
			SvId:      23,
		},
		{
			Bitfield1: SecOsnmaAuthSVBitfield(1023 | (31 << 10)), // IODE=1023, AuthNum=31, AuthStatus=success
			SvId:      7,
		},
	}

	// Manually construct the expected byte sequence for UBX-SEC-OSNMA v3
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x27, 0x0a, // class=SEC(0x27), id=OSNMA(0x0a)
		0x24, 0x00, // length = 36 bytes (28 fixed + 2*4 AuthSVs)
		// Payload starts here:
		msgVersion,                                        // version = 0x03
		byte(nmaHeader),                                   // nmaHeader
		byte(osnmaMonitoring), byte(osnmaMonitoring >> 8), // osnmaMonitoring (little endian)
		byte(timSyncReq), // timSyncReq
		0x00, 0x00, 0x00, // reserved0[3]
		// timSyncReqDiff (little endian int32)
		byte(timSyncReqDiff & 0xFF), byte((timSyncReqDiff >> 8) & 0xFF), byte((timSyncReqDiff >> 16) & 0xFF), byte((timSyncReqDiff >> 24) & 0xFF),
		0x00, 0x00, 0x00, 0x00, // reserved1[4]
		// dsmAuthentication (little endian uint32)
		byte(dsmAuthentication), byte(dsmAuthentication >> 8), byte(dsmAuthentication >> 16), byte(dsmAuthentication >> 24),
		// teslaKey (little endian uint32)
		byte(teslaKey), byte(teslaKey >> 8), byte(teslaKey >> 16), byte(teslaKey >> 24),
		// generalAndTiming (little endian uint32)
		byte(generalAndTiming), byte(generalAndTiming >> 8), byte(generalAndTiming >> 16), byte(generalAndTiming >> 24),
		// AuthSVs[0]
		byte(authSVs[0].Bitfield1), byte(authSVs[0].Bitfield1 >> 8), // bitfield1 (little endian)
		authSVs[0].SvId, 0x00, // svId, reserved2
		// AuthSVs[1]
		byte(authSVs[1].Bitfield1), byte(authSVs[1].Bitfield1 >> 8), // bitfield1 (little endian)
		authSVs[1].SvId, 0x00, // svId, reserved2
	}

	// Calculate and append checksum
	ckA, ckB := Checksum(packet[2:]) // Skip sync bytes for checksum
	packet = append(packet, ckA, ckB)

	// Parse the message
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse SEC-OSNMA v3: %v", err)
	}

	// Verify it's the right type
	secOsnma, ok := msg.(*SecOsnma)
	if !ok {
		t.Fatalf("expected *SecOsnma, got %T", msg)
	}

	// Check version
	if secOsnma.Version != msgVersion {
		t.Errorf("expected Version=0x%02x, got 0x%02x", msgVersion, secOsnma.Version)
	}

	// Check fixed part fields directly
	v3 := &secOsnma.SecOsnmaFixed

	// Verify all fields
	if v3.NmaHeader != nmaHeader {
		t.Errorf("expected NmaHeader=0x%02x, got 0x%02x", nmaHeader, v3.NmaHeader)
	}
	if v3.OsnmaMonitoring != osnmaMonitoring {
		t.Errorf("expected OsnmaMonitoring=0x%04x, got 0x%04x", osnmaMonitoring, v3.OsnmaMonitoring)
	}
	if v3.TimSyncReq != timSyncReq {
		t.Errorf("expected TimSyncReq=0x%02x, got 0x%02x", timSyncReq, v3.TimSyncReq)
	}
	if v3.TimSyncReqDiff != timSyncReqDiff {
		t.Errorf("expected TimSyncReqDiff=%d, got %d", timSyncReqDiff, v3.TimSyncReqDiff)
	}
	if v3.DsmAuthentication != dsmAuthentication {
		t.Errorf("expected DsmAuthentication=0x%08x, got 0x%08x", dsmAuthentication, v3.DsmAuthentication)
	}
	if v3.TeslaKey != teslaKey {
		t.Errorf("expected TeslaKey=0x%08x, got 0x%08x", teslaKey, v3.TeslaKey)
	}
	if v3.GeneralAndTiming != generalAndTiming {
		t.Errorf("expected GeneralAndTiming=0x%08x, got 0x%08x", generalAndTiming, v3.GeneralAndTiming)
	}

	// Check AuthSVs array
	if len(secOsnma.AuthSVs) != len(authSVs) {
		t.Fatalf("expected %d AuthSVs, got %d", len(authSVs), len(secOsnma.AuthSVs))
	}
	for i, expected := range authSVs {
		actual := secOsnma.AuthSVs[i]
		if actual.Bitfield1 != expected.Bitfield1 {
			t.Errorf("AuthSVs[%d]: expected Bitfield1=0x%04x, got 0x%04x", i, expected.Bitfield1, actual.Bitfield1)
		}
		if actual.SvId != expected.SvId {
			t.Errorf("AuthSVs[%d]: expected SvId=%d, got %d", i, expected.SvId, actual.SvId)
		}
	}

	// Test bitfield extraction methods by testing constants
	if (v3.NmaHeader & SecOsnmaHeaderAuthStatus) == 0 != !headerAuthStatus {
		t.Errorf("NmaHeader auth status extraction failed")
	}
	if (v3.NmaHeader>>1)&0x03 != nmaStatus {
		t.Errorf("NmaHeader status extraction failed")
	}
	if (v3.OsnmaMonitoring & SecOsnmaMonitoringOsnmaEnabled) == 0 != !osnmaEnabled {
		t.Errorf("OsnmaMonitoring enabled extraction failed")
	}

	// Test serialization roundtrip
	newMsg := &SecOsnma{
		SecOsnmaFixed: SecOsnmaFixed{
			Version:           msgVersion,
			NmaHeader:         nmaHeader,
			OsnmaMonitoring:   osnmaMonitoring,
			TimSyncReq:        timSyncReq,
			TimSyncReqDiff:    timSyncReqDiff,
			DsmAuthentication: dsmAuthentication,
			TeslaKey:          teslaKey,
			GeneralAndTiming:  generalAndTiming,
		},
		AuthSVs: authSVs,
	}

	serialized, err := Serialize(newMsg)
	if err != nil {
		t.Fatalf("failed to serialize SEC-OSNMA v3: %v", err)
	}

	if !slices.Equal(serialized, packet) {
		t.Errorf("serialization mismatch:\nexpected: %x\ngot:      %x", packet, serialized)
	}
}

func TestSecOsnmaUnknownVersion(t *testing.T) {
	// Test that unknown version gets parsed as UnknownMsg
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x27, 0x0a, // class=SEC(0x27), id=OSNMA(0x0a)
		0x20, 0x00, // length = 32 bytes
		// Payload starts here:
		0x04,       // version = 0x04 (unknown)
		0x01,       // nmaHeader
		0x02, 0x03, // osnmaMonitoring
		0x04,             // timSyncReq
		0x00, 0x00, 0x00, // reserved0
		0x05, 0x06, 0x07, 0x08, // timSyncReqDiff
		0x00, 0x00, 0x00, 0x00, // reserved1
		0x09, 0x0A, 0x0B, 0x0C, // dsmAuthentication
		0x0D, 0x0E, 0x0F, 0x10, // teslaKey
		0x11, 0x12, 0x13, 0x14, // generalAndTiming
		0x15, 0x16, 0x17, 0x18, // extra data
	}

	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)

	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse SEC-OSNMA v4: %v", err)
	}

	// Should be parsed as UnknownMsg
	unknownMsg, ok := msg.(*UnknownMsg)
	if !ok {
		t.Fatalf("expected *UnknownMsg for unknown version, got %T", msg)
	}

	if unknownMsg.MsgID != SecOsnmaID {
		t.Errorf("expected MsgID=%v, got %v", SecOsnmaID, unknownMsg.MsgID)
	}
}

func TestSecOsnmaEmpty(t *testing.T) {
	// Test minimal v3 message with no AuthSVs
	const msgVersion = 0x03

	// Manually construct minimal packet (28 bytes payload)
	packet := []byte{
		0xB5, 0x62, // sync bytes
		0x27, 0x0a, // class=SEC(0x27), id=OSNMA(0x0a)
		0x1C, 0x00, // length = 28 bytes (minimal for v3)
		// Payload starts here:
		msgVersion, // version = 0x03
		0x00,       // nmaHeader = 0
		0x00, 0x00, // osnmaMonitoring = 0
		0x00,             // timSyncReq = 0
		0x00, 0x00, 0x00, // reserved0[3]
		0x00, 0x00, 0x00, 0x00, // timSyncReqDiff = 0
		0x00, 0x00, 0x00, 0x00, // reserved1[4]
		0x00, 0x00, 0x00, 0x00, // dsmAuthentication = 0
		0x00, 0x00, 0x00, 0x00, // teslaKey = 0
		0x00, 0x00, 0x00, 0x00, // generalAndTiming = 0 (authSVsCount=0)
		// No AuthSVs
	}

	ckA, ckB := Checksum(packet[2:])
	packet = append(packet, ckA, ckB)

	// Parse the message
	msg, err := ParseMsg(string(packet))
	if err != nil {
		t.Fatalf("failed to parse minimal SEC-OSNMA v3: %v", err)
	}

	secOsnma, ok := msg.(*SecOsnma)
	if !ok {
		t.Fatalf("expected *SecOsnma, got %T", msg)
	}

	if secOsnma.Version != msgVersion {
		t.Errorf("expected Version=0x%02x, got 0x%02x", msgVersion, secOsnma.Version)
	}

	if len(secOsnma.AuthSVs) != 0 {
		t.Errorf("expected 0 AuthSVs, got %d", len(secOsnma.AuthSVs))
	}

	// Test round-trip using testMsgType1 since SecOsnma has slices and isn't comparable
	p2 := testMsgType1(t, *secOsnma)
	secOsnma2 := p2.(*SecOsnma)
	if secOsnma2.Version != secOsnma.Version {
		t.Errorf("round-trip failed: version mismatch")
	}
}
