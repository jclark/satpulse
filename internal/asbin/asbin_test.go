package asbin

import (
	"testing"
)

func TestCfgPpsExample(t *testing.T) {
	// Example from spec:
	// In cynosure II, to set 1PPS with pulse length 500us and positive polarity high on GPIO13 with PPS
	// output even there is no position fix.
	// F1 D9 06 07 0F 00 40 42 0F 00 00 00 00 00 10 27 00 00 01 0D 01 F3 86
	packet := "\xF1\xD9\x06\x07\x0F\x00\x40\x42\x0F\x00\x00\x00\x00\x00\x10\x27\x00\x00\x01\x0D\x01\xF3\x86"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgPps: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgPpsID {
		t.Errorf("Expected message ID %v, got %v", CfgPpsID, msg.ID())
	}

	cfgPps, ok := msg.(*CfgPps)
	if !ok {
		t.Fatalf("Expected *CfgPps, got %T", msg)
	}

	// Verify fields based on the example:
	// 40 42 0F 00 = 1000000 (0x0F4240) in little-endian = 1,000,000 µs = 1 second period
	if cfgPps.Period != 1000000 {
		t.Errorf("Expected Period = 1000000 µs (1s), got %d µs", cfgPps.Period)
	}

	// 00 00 00 00 = 0 ns offset
	if cfgPps.Offset != 0 {
		t.Errorf("Expected Offset = 0 ns, got %d ns", cfgPps.Offset)
	}

	// 10 27 00 00 = 10000 (0x2710) in little-endian = 10% duty cycle
	// Where 10^6 (1000000) would be 100%
	if cfgPps.DutyCycle != 10000 {
		t.Errorf("Expected DutyCycle = 10000 (10%%), got %d", cfgPps.DutyCycle)
	}

	// 01 = Polarity: rising edge (positive)
	if cfgPps.Polarity != CfgPpsPolarityRisingEdge {
		t.Errorf("Expected Polarity = %d (rising edge), got %d", CfgPpsPolarityRisingEdge, cfgPps.Polarity)
	}

	// 0D = GPIO 13
	if cfgPps.GPIO != 13 {
		t.Errorf("Expected GPIO = 13, got %d", cfgPps.GPIO)
	}

	// 01 = Always output (even when no fix)
	if cfgPps.Sync != CfgPpsSyncAlways {
		t.Errorf("Expected Sync = %d (always output), got %d", CfgPpsSyncAlways, cfgPps.Sync)
	}
}

func TestCfgCfgBaudrateNmea(t *testing.T) {
	// Write baudrate and NMEA message rate configuration into involatile memory
	// F1 D9 06 09 08 00 00 00 00 00 03 00 00 00 1A 07
	packet := "\xF1\xD9\x06\x09\x08\x00\x00\x00\x00\x00\x03\x00\x00\x00\x1A\x07"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgCfg: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgCfgID {
		t.Errorf("Expected message ID %v, got %v", CfgCfgID, msg.ID())
	}

	cfgCfg, ok := msg.(*CfgCfg)
	if !ok {
		t.Fatalf("Expected *CfgCfg, got %T", msg)
	}

	// Verify fields based on the example:
	// 00 00 00 00 = Action: Save (0)
	if cfgCfg.Action != CfgCfgActionSave {
		t.Errorf("Expected Action = %d (Save), got %d", CfgCfgActionSave, cfgCfg.Action)
	}

	// 03 00 00 00 = Mask: 0x03 = Baudrate(0x01) + NMEA msg rate(0x02)
	expectedMask := CfgCfgMaskBaudrate | CfgCfgMaskNmeaMsgRate
	if cfgCfg.Mask != expectedMask {
		t.Errorf("Expected Mask = 0x%X, got 0x%X", expectedMask, cfgCfg.Mask)
	}
}

func TestCfgCfgNavSettings(t *testing.T) {
	// ECEF position, ephemeris saving into involatile memory
	// F1 D9 06 09 08 00 00 00 00 00 04 00 00 00 1B 0B
	packet := "\xF1\xD9\x06\x09\x08\x00\x00\x00\x00\x00\x04\x00\x00\x00\x1B\x0B"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgCfg: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgCfgID {
		t.Errorf("Expected message ID %v, got %v", CfgCfgID, msg.ID())
	}

	cfgCfg, ok := msg.(*CfgCfg)
	if !ok {
		t.Fatalf("Expected *CfgCfg, got %T", msg)
	}

	// Verify fields based on the example:
	// 00 00 00 00 = Action: Save (0)
	if cfgCfg.Action != CfgCfgActionSave {
		t.Errorf("Expected Action = %d (Save), got %d", CfgCfgActionSave, cfgCfg.Action)
	}

	// 04 00 00 00 = Mask: 0x04 = Navigation settings (bit 2 set)
	if cfgCfg.Mask != CfgCfgMaskNavSettings {
		t.Errorf("Expected Mask = 0x%X (NavSettings), got 0x%X", CfgCfgMaskNavSettings, cfgCfg.Mask)
	}
}

func TestCfgCfgFactoryReset(t *testing.T) {
	// Factory reset:
	// F1 D9 06 09 08 00 02 00 00 00 FF FF FF FF 15 01
	packet := "\xF1\xD9\x06\x09\x08\x00\x02\x00\x00\x00\xFF\xFF\xFF\xFF\x15\x01"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgCfg: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgCfgID {
		t.Errorf("Expected message ID %v, got %v", CfgCfgID, msg.ID())
	}

	cfgCfg, ok := msg.(*CfgCfg)
	if !ok {
		t.Fatalf("Expected *CfgCfg, got %T", msg)
	}

	// Verify fields based on the example:
	// 02 00 00 00 = Action: Clear (2)
	if cfgCfg.Action != CfgCfgActionClear {
		t.Errorf("Expected Action = %d (Clear), got %d", CfgCfgActionClear, cfgCfg.Action)
	}

	// FF FF FF FF = Mask: 0xFFFFFFFF = Factory reset
	if cfgCfg.Mask != CfgCfgMaskFactoryReset {
		t.Errorf("Expected Mask = 0x%X (Factory reset), got 0x%X", CfgCfgMaskFactoryReset, cfgCfg.Mask)
	}
}

func TestNavTime(t *testing.T) {
	// Example from spec:
	// Get the GPS time
	// F1 D9 01 05 10 00 00 07 2C 79 FF 55 3E 16 10 00 12 00 06 00 00 00 92 5A
	packet := "\xF1\xD9\x01\x05\x10\x00\x00\x07\x2C\x79\xFF\x55\x3E\x16\x10\x00\x12\x00\x06\x00\x00\x00\x92\x5A"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse NavTime: %v", err)
	}

	// Verify message type
	if msg.ID() != NavTimeID {
		t.Errorf("Expected message ID %v, got %v", NavTimeID, msg.ID())
	}

	navTime, ok := msg.(*NavTime)
	if !ok {
		t.Fatalf("Expected *NavTime, got %T", msg)
	}

	// Verify fields based on the example:
	// NavSys = 0 (GPS)
	if navTime.NavSys != NavTimeSysGPS {
		t.Errorf("Expected NavSys = GPS(0), got %d", navTime.NavSys)
	}

	// Flags = 7 (all bits set for week, second, leapsecond valid)
	expectedFlags := NavTimeFlagWeekValid | NavTimeFlagSecondValid | NavTimeFlagLeapSecValid
	if navTime.Flags != expectedFlags {
		t.Errorf("Expected Flags = %d, got %d", expectedFlags, navTime.Flags)
	}

	// 2C 79 = 31020 (little-endian) = 31020 ns
	if navTime.Fractow != 31020 {
		t.Errorf("Expected Fractow = 31020, got %d", navTime.Fractow)
	}

	// FF 55 3E 16 = 0x163E55FF (little-endian) = 373183999 ms
	if navTime.RefTow != 373183999 {
		t.Errorf("Expected RefTow = 373183999, got %d", navTime.RefTow)
	}

	// 10 00 12 00 = 16 (little-endian) - Week number after the April 2019 GPS rollover
	// This corresponds to approximately July/August 2019
	if navTime.Week != 16 {
		t.Errorf("Expected Week = 16, got %d", navTime.Week)
	}

	// 06 00 = 18 (little-endian) - Correct leap seconds for mid-2019
	// GPS-UTC offset was 18 seconds after the leap second added on December 31, 2016
	if navTime.LeapSec != 18 {
		t.Errorf("Expected LeapSec = 18, got %d", navTime.LeapSec)
	}

	// 00 00 00 00 = 6 (decimal) when properly parsed
	if navTime.TimeErr != 6 {
		t.Errorf("Expected TimeErr = 6, got %d", navTime.TimeErr)
	}
}

func TestPollNavTime(t *testing.T) {
	// Test creating a poll message for NAV-TIME
	expectedStr := "\xF1\xD9\x01\x05\x01\x00\x00\x07\x1C"

	// Generate the poll message
	pollMsg := PollNavTime(NavTimeSysGPS)

	// Convert pollMsg byte slice to string for comparison
	pollMsgStr := string(pollMsg)

	// Compare the generated message with the expected one
	if pollMsgStr != expectedStr {
		// Display the hex bytes for easier debugging
		expectedBytes := []byte(expectedStr)
		t.Errorf("Poll message for NAV-TIME doesn't match expected bytes.\nExpected: %X\nGot:      %X", expectedBytes, pollMsg)
	}

	// Verify that the message ID is correctly extracted
	extractedID := PacketMsgId(pollMsg)
	if extractedID != NavTimeID {
		t.Errorf("Expected message ID %v, got %v", NavTimeID, extractedID)
	}
}

func TestAckNak(t *testing.T) {
	// Example from spec:
	// Receiver NOT acknowledge message CFG-MSG which include invalid payload content
	// (GroupID: 0x06, SubID: 0x01)
	// F1 D9 05 00 02 00 06 01 0E 33
	packet := "\xF1\xD9\x05\x00\x02\x00\x06\x01\x0E\x33"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse AckNak: %v", err)
	}

	// Verify message type
	if msg.ID() != AckNakID {
		t.Errorf("Expected message ID %v, got %v", AckNakID, msg.ID())
	}

	ackNak, ok := msg.(*AckNak)
	if !ok {
		t.Fatalf("Expected *AckNak, got %T", msg)
	}

	// Verify fields based on the example:
	// 06 = GroupID / Class (CFG)
	if ackNak.MsgClass != 0x06 {
		t.Errorf("Expected ClsID[0] = 0x06 (CFG), got 0x%02X", ackNak.MsgClass)
	}

	// 01 = MsgID / SubID (MSG)
	if ackNak.MsgID != 0x01 {
		t.Errorf("Expected ClsID[1] = 0x01 (MSG), got 0x%02X", ackNak.MsgID)
	}
}

func TestAckAck(t *testing.T) {
	// Example from spec:
	// Receiver acknowledge message CFG-SIMPLERST (GroupID: 0x06, SubID: 0x40)
	// F1 D9 05 01 02 00 06 40 4E 77
	packet := "\xF1\xD9\x05\x01\x02\x00\x06\x40\x4E\x77"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse AckAck: %v", err)
	}

	// Verify message type
	if msg.ID() != AckAckID {
		t.Errorf("Expected message ID %v, got %v", AckAckID, msg.ID())
	}

	ackAck, ok := msg.(*AckAck)
	if !ok {
		t.Fatalf("Expected *AckAck, got %T", msg)
	}

	// Verify fields based on the example:
	// 06 = GroupID / Class (CFG)
	if ackAck.MsgClass != 0x06 {
		t.Errorf("Expected MsgClass = 0x06 (CFG), got 0x%02X", ackAck.MsgClass)
	}

	// 40 = MsgID / SubID (SIMPLERST)
	if ackAck.MsgID != 0x40 {
		t.Errorf("Expected MsgID = 0x40 (SIMPLERST), got 0x%02X", ackAck.MsgID)
	}

	// Verify this is acknowledging a CFG-SIMPLERST message
	mid := makeMsgID(ackAck.MsgClass, ackAck.MsgID)
	if mid != CfgSimpleRstID {
		t.Errorf("Expected acknowledged message ID to be CFG-SIMPLERST, got %v",
			mid)
	}
}

func TestCfgPrtUART1At9600(t *testing.T) {
	// Example from spec:
	// To set UART1 at 9600 baud
	// F1 D9 06 00 08 00 01 00 00 00 80 25 00 00 B4 0F
	packet := "\xF1\xD9\x06\x00\x08\x00\x01\x00\x00\x00\x80\x25\x00\x00\xB4\x0F"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgPrt: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgPrtID {
		t.Errorf("Expected message ID %v, got %v", CfgPrtID, msg.ID())
	}

	cfgPrt, ok := msg.(*CfgPrt)
	if !ok {
		t.Fatalf("Expected *CfgPrt, got %T", msg)
	}

	// Verify fields based on the example:
	// 01 = PortID: 1 (UART1)
	if cfgPrt.PortID != 1 {
		t.Errorf("Expected PortID = 1 (UART1), got %d", cfgPrt.PortID)
	}

	// Res is just 3 reserved bytes [00 00 00]
	for i, v := range cfgPrt.Res {
		if v != 0 {
			t.Errorf("Expected Res[%d] = 0, got %d", i, v)
		}
	}

	// 80 25 00 00 = 9600 (little-endian)
	expectedBaudRate := uint32(9600)
	if cfgPrt.Baudrate != expectedBaudRate {
		t.Errorf("Expected Baudrate = %d, got %d", expectedBaudRate, cfgPrt.Baudrate)
	}
}

func TestCfgPrtUART0At115200(t *testing.T) {
	// Example from spec:
	// To set UART0 at 115200 baud
	// F1 D9 06 00 08 00 00 00 00 00 00 C2 01 00 D1 E0
	packet := "\xF1\xD9\x06\x00\x08\x00\x00\x00\x00\x00\x00\xC2\x01\x00\xD1\xE0"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgPrt: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgPrtID {
		t.Errorf("Expected message ID %v, got %v", CfgPrtID, msg.ID())
	}

	cfgPrt, ok := msg.(*CfgPrt)
	if !ok {
		t.Fatalf("Expected *CfgPrt, got %T", msg)
	}

	// Verify fields based on the example:
	// 00 = PortID: 0 (UART0)
	if cfgPrt.PortID != 0 {
		t.Errorf("Expected PortID = 0 (UART0), got %d", cfgPrt.PortID)
	}

	// Res is just 3 reserved bytes [00 00 00]
	for i, v := range cfgPrt.Res {
		if v != 0 {
			t.Errorf("Expected Res[%d] = 0, got %d", i, v)
		}
	}

	// 00 C2 01 00 = 115200 (little-endian)
	expectedBaudRate := uint32(115200)
	if cfgPrt.Baudrate != expectedBaudRate {
		t.Errorf("Expected Baudrate = %d, got %d", expectedBaudRate, cfgPrt.Baudrate)
	}
}

func TestPollPrt(t *testing.T) {
	// Test creating a poll message for CFG-PRT (UART0)
	// Example from spec:
	// Poll current UART0 configuration
	// F1 D9 06 00 01 00 01 07 21
	expectedStr := "\xF1\xD9\x06\x00\x01\x00\x00\x07\x21"

	// Generate the poll message
	pollMsg := PollPrt(0) // UART0

	// Convert pollMsg byte slice to string for comparison
	pollMsgStr := string(pollMsg)

	// Compare the generated message with the expected one
	if pollMsgStr != expectedStr {
		// Display the hex bytes for easier debugging
		expectedBytes := []byte(expectedStr)
		t.Errorf("Poll message for CFG-PRT doesn't match expected bytes.\nExpected: %X\nGot:      %X", expectedBytes, pollMsg)
	}

	// Verify that the message ID is correctly extracted
	extractedID := PacketMsgId(pollMsg)
	if extractedID != CfgPrtID {
		t.Errorf("Expected message ID %v, got %v", CfgPrtID, extractedID)
	}
}

func TestCfgMsgSetNmeaGsvRate(t *testing.T) {
	// Example from spec:
	// Set NMEA GSV message rate to 1 per 2 seconds
	// F1 D9 06 01 03 00 F0 04 02 00 19
	packet := "\xF1\xD9\x06\x01\x03\x00\xF0\x04\x02\x00\x19"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgMsg: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgMsgID {
		t.Errorf("Expected message ID %v, got %v", CfgMsgID, msg.ID())
	}

	cfgMsg, ok := msg.(*CfgMsg)
	if !ok {
		t.Fatalf("Expected *CfgMsg, got %T", msg)
	}

	// Verify fields based on the example:

	// 02 = Rate (1 per 2 seconds)
	if cfgMsg.Rate != 2 {
		t.Errorf("Expected Rate = 2 (1 per 2 seconds), got %d", cfgMsg.Rate)
	}

	// Verify this corresponds to NmeaGsvID
	expectedMsgID := makeMsgID(cfgMsg.MsgClass, cfgMsg.MsgID)
	if expectedMsgID != NmeaGsvID {
		t.Errorf("Expected message to configure NmeaGsvID, got %v", expectedMsgID)
	}
}

func TestCfgMsgSetNmeaGllRate(t *testing.T) {
	// Example from spec:
	// Set NMEA GLL message rate to 1 per 5 seconds
	// F1 D9 06 01 03 00 F0 01 05 00 16
	packet := "\xF1\xD9\x06\x01\x03\x00\xF0\x01\x05\x00\x16"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgMsg: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgMsgID {
		t.Errorf("Expected message ID %v, got %v", CfgMsgID, msg.ID())
	}

	cfgMsg, ok := msg.(*CfgMsg)
	if !ok {
		t.Fatalf("Expected *CfgMsg, got %T", msg)
	}

	// Verify fields based on the example:

	if cfgMsg.Rate != 5 {
		t.Errorf("Expected Rate = 5 (1 per 5 seconds), got %d", cfgMsg.Rate)
	}

	// Verify this corresponds to NmeaGllID
	expectedMsgID := makeMsgID(cfgMsg.MsgClass, cfgMsg.MsgID)
	if expectedMsgID != NmeaGllID {
		t.Errorf("Expected message to configure NmeaGllID, got %v", expectedMsgID)
	}
}

func TestCfgMsgDisableNmeaVtg(t *testing.T) {
	// Example from spec:
	// Disable NMEA VTG message
	// F1 D9 06 01 03 00 F0 06 00 00 1B
	packet := "\xF1\xD9\x06\x01\x03\x00\xF0\x06\x00\x00\x1B"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgMsg: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgMsgID {
		t.Errorf("Expected message ID %v, got %v", CfgMsgID, msg.ID())
	}

	cfgMsg, ok := msg.(*CfgMsg)
	if !ok {
		t.Fatalf("Expected *CfgMsg, got %T", msg)
	}
	// Verify fields based on the example:
	if cfgMsg.Rate != 0 {
		t.Errorf("Expected Rate = 0 (disabled), got %d", cfgMsg.Rate)
	}

	// Verify this corresponds to NmeaVtgID
	expectedMsgID := makeMsgID(cfgMsg.MsgClass, cfgMsg.MsgID)
	if expectedMsgID != NmeaVtgID {
		t.Errorf("Expected message to configure NmeaVtgID, got %v", expectedMsgID)
	}
}

func TestPollCfgMsgNmeaGga(t *testing.T) {
	// Example from spec:
	// Poll current message rate of NMEA GGA message
	// F1 D9 06 01 02 00 F0 00 F9 11
	expectedStr := "\xF1\xD9\x06\x01\x02\x00\xF0\x00\xF9\x11"

	// Generate the poll message
	pollMsg := PollCfgMsg(NmeaGgaID)

	// Convert pollMsg byte slice to string for comparison
	pollMsgStr := string(pollMsg)

	// Compare the generated message with the expected one
	if pollMsgStr != expectedStr {
		// Display the hex bytes for easier debugging
		expectedBytes := []byte(expectedStr)
		t.Errorf("Poll message for NMEA GGA doesn't match expected bytes.\nExpected: %X\nGot:      %X", expectedBytes, pollMsg)
	}

	// Verify that the message ID is correctly extracted
	extractedID := PacketMsgId(pollMsg)
	if extractedID != CfgMsgID {
		t.Errorf("Expected message ID %v, got %v", CfgMsgID, extractedID)
	}
}

func TestCfgNavSat(t *testing.T) {
	// Example from spec:
	// Set to use GPS L1, BEIDOU B1, GPS L5, BEIDOU B2A
	// F1 D9 06 0C 04 00 05 82 00 00 9D 36
	packet := "\xF1\xD9\x06\x0C\x04\x00\x05\x82\x00\x00\x9D\x36"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgNavSat: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgNavSatID {
		t.Errorf("Expected message ID %v, got %v", CfgNavSatID, msg.ID())
	}

	cfgNavSat, ok := msg.(*CfgNavSat)
	if !ok {
		t.Fatalf("Expected *CfgNavSat, got %T", msg)
	}

	// Verify fields based on the example:

	expectedMask := CfgNavSatMaskGPSL1 | CfgNavSatMaskBEIDOUB1 | CfgNavSatMaskGPSL5 | CfgNavSatMaskBEIDOUB2A

	if cfgNavSat.EnableMask != expectedMask {
		t.Errorf("Expected EnableMask = 0x%08X, got 0x%08X", expectedMask, cfgNavSat.EnableMask)
	}
}

func TestCfgSurvey(t *testing.T) {
	// Example from spec:
	// Set survey time to 5s, accuracy requirement to 100mm
	// F1 D9 06 12 08 00 05 00 00 00 64 00 00 00 89 16
	packet := "\xF1\xD9\x06\x12\x08\x00\x05\x00\x00\x00\x64\x00\x00\x00\x89\x16"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgSurvey: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgSurveyID {
		t.Errorf("Expected message ID %v, got %v", CfgSurveyID, msg.ID())
	}

	cfgSurvey, ok := msg.(*CfgSurvey)
	if !ok {
		t.Fatalf("Expected *CfgSurvey, got %T", msg)
	}

	// Verify fields based on the example:
	if cfgSurvey.MinDur != 5 {
		t.Errorf("Expected MinDur = 5 seconds, got %d seconds", cfgSurvey.MinDur)
	}

	if cfgSurvey.AccLimit != 100 {
		t.Errorf("Expected AccLimit = 100 mm, got %d mm", cfgSurvey.AccLimit)
	}
}

func TestCfgSimpleRstWarmStart(t *testing.T) {
	// Example from spec:
	// Configure a warm start (as system command, there is NO ACK)
	// F1 D9 06 40 01 00 02 49 23
	packet := "\xF1\xD9\x06\x40\x01\x00\x02\x49\x23"

	msg, err := ParseMsg(packet)
	if err != nil {
		t.Fatalf("Failed to parse CfgSimpleRst: %v", err)
	}

	// Verify message type
	if msg.ID() != CfgSimpleRstID {
		t.Errorf("Expected message ID %v, got %v", CfgSimpleRstID, msg.ID())
	}

	cfgSimpleRst, ok := msg.(*CfgSimpleRst)
	if !ok {
		t.Fatalf("Expected *CfgSimpleRst, got %T", msg)
	}

	// Verify fields based on the example:
	if cfgSimpleRst.Mode != CfgSimpleRstModeWarmStart {
		t.Errorf("Expected Mode = %d (Warm start), got %d", CfgSimpleRstModeWarmStart, cfgSimpleRst.Mode)
	}

}

func TestMsgIDString(t *testing.T) {
	tests := []struct {
		mid  MsgID
		want string
	}{
		// Messages with decoders
		{NavTimeID, "NAV-TIME"},
		{NavClockID, "NAV-CLOCK"},
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
