package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func TestProcessPacketWithParseError(t *testing.T) {
	p := NewPacketProcessor(gpsprot.NewNavEpochManager())

	// Create a valid UBX packet with empty payload for CFG-NAV5 (which requires 36 bytes)
	// This will pass PacketFormat validation but fail during ParseMsg
	invalidPacket, err := ubxbin.PackMsg(ubxbin.CfgNav5ID, []byte{})
	if err != nil {
		t.Fatalf("failed to create test packet: %v", err)
	}

	var tZero time.Time // Use zero time for simplicity
	msgID, err := p.ProcessPacket(string(invalidPacket), tZero)

	// Should return error from ParseMsg
	if err == nil {
		t.Fatal("expected error when ParseMsg fails, got nil")
	}

	// msgID should be from PacketFormat.MsgID when m is nil
	expectedMsgID := ubxbin.CfgNav5ID.String()
	if msgID != expectedMsgID {
		t.Fatalf("expected msgID %q when ParseMsg fails, got %q", expectedMsgID, msgID)
	}
}

// Bit flags for expected messages
const (
	testExpectTime       = 1 << 0 // TimeMsg expected
	testExpectSatellites = 1 << 1 // SatellitesMsg expected
	testExpectLeapSecond = 1 << 2 // LeapSecondMsg expected
	testExpectSurvey     = 1 << 3 // SurveyMsg expected
	testExpectNavEpoch   = 1 << 4 // NavEpochMsg expected
)

type testNavEpochTestCase struct {
	name  string
	steps []testNavEpochStep
}

type testNavEpochStep struct {
	// Input
	msgID    ubxbin.MsgID // Message type being processed
	navEpoch uint32       // The iTOW value for this message
	tRead    time.Time    // When the message was received

	// Expected outputs (bit flags)
	expectMsgs int // Bit flags for expected messages
}

type testMsgHandler struct {
	gpsprot.DefaultHandler
	msgs []struct {
		msgType string
		msg     interface{}
		tRead   time.Time
	}
}

func (h *testMsgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     interface{}
		tRead   time.Time
	}{"time", msg, tRead})
}

func (h *testMsgHandler) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     interface{}
		tRead   time.Time
	}{"satellites", msg, tRead})
}

func (h *testMsgHandler) LeapSecond(msg *gpsprot.LeapSecondMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     interface{}
		tRead   time.Time
	}{"leapsecond", msg, tRead})
}

func (h *testMsgHandler) Survey(msg *gpsprot.SurveyMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     interface{}
		tRead   time.Time
	}{"survey", msg, tRead})
}

func (h *testMsgHandler) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     interface{}
		tRead   time.Time
	}{"navepoch", msg, tRead})
}

func TestNavEpochHandling(t *testing.T) {
	tests := []testNavEpochTestCase{
		{
			name: "complete_epoch_flushing",
			steps: []testNavEpochStep{
				// We can always flush when we have both NAV-SAT and NAV-SIG
				{msgID: ubxbin.NavSatID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: 0},
				{msgID: ubxbin.NavSigID, navEpoch: 100, tRead: time.Unix(2, 0), expectMsgs: testExpectSatellites},
				{msgID: ubxbin.NavSatID, navEpoch: 200, tRead: time.Unix(3, 0), expectMsgs: testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 200, tRead: time.Unix(4, 0), expectMsgs: testExpectSatellites},
				{msgID: ubxbin.NavSatID, navEpoch: 300, tRead: time.Unix(5, 0), expectMsgs: testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 300, tRead: time.Unix(6, 0), expectMsgs: testExpectSatellites},
				{msgID: ubxbin.NavSatID, navEpoch: 400, tRead: time.Unix(7, 0), expectMsgs: testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 400, tRead: time.Unix(8, 0), expectMsgs: testExpectSatellites},
			},
		},
		{
			name: "nav_sat_only",
			steps: []testNavEpochStep{
				// Epoch 1
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: testExpectTime},
				{msgID: ubxbin.NavSatID, navEpoch: 100, tRead: time.Unix(2, 0), expectMsgs: 0},
				// Epoch 2
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 200, tRead: time.Unix(3, 0), expectMsgs: testExpectTime | testExpectSatellites | testExpectNavEpoch},
				{msgID: ubxbin.NavSatID, navEpoch: 200, tRead: time.Unix(4, 0), expectMsgs: 0},
				// Epoch  3
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 300, tRead: time.Unix(5, 0), expectMsgs: testExpectTime | testExpectSatellites | testExpectNavEpoch},
				// we have seen a complete epoch without NAV-SIG, so we can flush NAV-SAT without waiting for NAV-SIG
				{msgID: ubxbin.NavSatID, navEpoch: 300, tRead: time.Unix(6, 0), expectMsgs: testExpectSatellites},
				// Epoch  4 - everything is flushed right away
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 400, tRead: time.Unix(7, 0), expectMsgs: testExpectTime | testExpectNavEpoch},
				{msgID: ubxbin.NavSatID, navEpoch: 400, tRead: time.Unix(8, 0), expectMsgs: testExpectSatellites},
			},
		},
		{
			name: "nav_sig_only",
			steps: []testNavEpochStep{
				// Epoch 1
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: testExpectTime},
				{msgID: ubxbin.NavSigID, navEpoch: 100, tRead: time.Unix(2, 0), expectMsgs: 0},
				// Epoch 2
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 200, tRead: time.Unix(3, 0), expectMsgs: testExpectTime | testExpectSatellites | testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 200, tRead: time.Unix(4, 0), expectMsgs: 0},
				// Epoch  3
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 300, tRead: time.Unix(5, 0), expectMsgs: testExpectTime | testExpectSatellites | testExpectNavEpoch},
				// we have seen a complete epoch without NAV-SAT, so we can flush NAV-SIG without waiting for NAV-SAT
				{msgID: ubxbin.NavSigID, navEpoch: 300, tRead: time.Unix(6, 0), expectMsgs: testExpectSatellites},
				// Epoch  4 - everything is flushed right away
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 400, tRead: time.Unix(7, 0), expectMsgs: testExpectTime | testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 400, tRead: time.Unix(8, 0), expectMsgs: testExpectSatellites},
			},
		},
		{
			name: "basic_epoch_flushing",
			steps: []testNavEpochStep{
				// Epoch 1 (potentially incomplete)
				{msgID: ubxbin.NavSatID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: 0},
				// Epoch 2 (first complete) - epoch 1 gets flushed when epoch 2 starts
				{msgID: ubxbin.NavSigID, navEpoch: 200, tRead: time.Unix(2, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch},
				// Epoch 3 - epoch 2 gets flushed when epoch 3 starts
				{msgID: ubxbin.NavSatID, navEpoch: 300, tRead: time.Unix(3, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 300, tRead: time.Unix(4, 0), expectMsgs: testExpectSatellites},
			},
		},
		{
			name: "nav_sig_before_nav_sat",
			steps: []testNavEpochStep{
				// Epoch 1 - NAV-SIG arrives before NAV-SAT
				{msgID: ubxbin.NavSigID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: 0},
				// Epoch 2 - other way round
				{msgID: ubxbin.NavSatID, navEpoch: 200, tRead: time.Unix(2, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch},
				// Epoch 3 - epoch 2 gets flushed when epoch 3 starts, then completed
				{msgID: ubxbin.NavSigID, navEpoch: 300, tRead: time.Unix(3, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch},
				{msgID: ubxbin.NavSatID, navEpoch: 300, tRead: time.Unix(4, 0), expectMsgs: testExpectSatellites},
			},
		},
		{
			name: "mixed_nav_messages",
			steps: []testNavEpochStep{
				// Epoch 1 - mix of NAV messages
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: testExpectTime},
				{msgID: ubxbin.NavSatID, navEpoch: 100, tRead: time.Unix(2, 0), expectMsgs: 0},
				// Epoch 2 - epoch 1 gets flushed, more mixed messages
				{msgID: ubxbin.NavSigID, navEpoch: 200, tRead: time.Unix(3, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch},
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 200, tRead: time.Unix(4, 0), expectMsgs: testExpectTime},
				// Epoch 3 - epoch 2 gets flushed when epoch 3 starts
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 300, tRead: time.Unix(5, 0), expectMsgs: testExpectTime | testExpectSatellites | testExpectNavEpoch},
				{msgID: ubxbin.NavSatID, navEpoch: 300, tRead: time.Unix(6, 0), expectMsgs: 0},
				{msgID: ubxbin.NavSigID, navEpoch: 300, tRead: time.Unix(7, 0), expectMsgs: testExpectSatellites},
			},
		},
		{
			name: "nav_eoe_flushes_epoch",
			steps: []testNavEpochStep{
				// Epoch 1 - NAV-PVT then NAV-EOE flushes immediately
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: testExpectTime},
				{msgID: ubxbin.NavEOEID, navEpoch: 100, tRead: time.Unix(2, 0), expectMsgs: testExpectNavEpoch},
				// Epoch 2 - no flush at epoch start since epoch 1 was already flushed by EOE
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 200, tRead: time.Unix(3, 0), expectMsgs: testExpectTime},
				{msgID: ubxbin.NavEOEID, navEpoch: 200, tRead: time.Unix(4, 0), expectMsgs: testExpectNavEpoch},
			},
		},
		{
			name: "nav_eoe_flushes_sats",
			steps: []testNavEpochStep{
				// Epoch 1 - NAV-SAT+NAV-SIG then NAV-EOE flushes sats and epoch
				{msgID: ubxbin.NavSatID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: 0},
				{msgID: ubxbin.NavSigID, navEpoch: 100, tRead: time.Unix(2, 0), expectMsgs: testExpectSatellites},
				{msgID: ubxbin.NavEOEID, navEpoch: 100, tRead: time.Unix(3, 0), expectMsgs: testExpectNavEpoch},
				// Epoch 2 - pending NAV-SAT is flushed by EOE (via FlushNavEpoch -> flushSats)
				{msgID: ubxbin.NavSatID, navEpoch: 200, tRead: time.Unix(4, 0), expectMsgs: 0},
				{msgID: ubxbin.NavEOEID, navEpoch: 200, tRead: time.Unix(5, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch},
			},
		},
		{
			name: "epoch_logic_after_three_epochs",
			steps: []testNavEpochStep{
				// Epoch 1 - complete epoch
				{msgID: ubxbin.NavSatID, navEpoch: 100, tRead: time.Unix(1, 0), expectMsgs: 0},
				// Epoch 2 - complete epoch, epoch 1 flushed
				{msgID: ubxbin.NavSigID, navEpoch: 200, tRead: time.Unix(2, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch},
				// Epoch 3 - complete epoch, epoch 2 flushed
				{msgID: ubxbin.NavSatID, navEpoch: 300, tRead: time.Unix(3, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 300, tRead: time.Unix(4, 0), expectMsgs: testExpectSatellites},
				// Epoch 4 - complete with NAV-SIG, now prevNavMsgs logic applies
				// Previous epoch (300) had NAV-SIG, so wait for NAV-SIG
				{msgID: ubxbin.NavSatID, navEpoch: 400, tRead: time.Unix(5, 0), expectMsgs: testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 400, tRead: time.Unix(6, 0), expectMsgs: testExpectSatellites},
				// Epoch 5 - previous epoch (400) had NAV-SIG, so wait for NAV-SIG
				{msgID: ubxbin.NavSatID, navEpoch: 500, tRead: time.Unix(7, 0), expectMsgs: testExpectNavEpoch},
				{msgID: ubxbin.NavSigID, navEpoch: 500, tRead: time.Unix(8, 0), expectMsgs: testExpectSatellites},
				// Epoch 6 - incomplete epoch (no NAV-SIG in previous epoch 500)
				// Previous epoch had NAV-SIG, so still wait for NAV-SIG
				{msgID: ubxbin.NavSatID, navEpoch: 600, tRead: time.Unix(9, 0), expectMsgs: testExpectNavEpoch},
				// No NAV-SIG for epoch 600
				// Epoch 7 - previous epoch (600) had NO NAV-SIG, so flush immediately with new epoch
				{msgID: ubxbin.NavTimeGPSID, navEpoch: 700, tRead: time.Unix(10, 0), expectMsgs: testExpectSatellites | testExpectNavEpoch | testExpectTime},
				{msgID: ubxbin.NavSatID, navEpoch: 700, tRead: time.Unix(11, 0), expectMsgs: testExpectSatellites},
				{msgID: ubxbin.NavSigID, navEpoch: 700, tRead: time.Unix(12, 0), expectMsgs: 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
			handler := &testMsgHandler{}
			pp.SetMsgHandler(handler)

			// Process steps and validate expected messages
			for i, step := range tt.steps {
				msgsBeforeCount := len(handler.msgs)

				// Create and serialize message
				msg := testCreateMessage(step.msgID, step.navEpoch)
				packet, err := ubxbin.Serialize(msg)
				if err != nil {
					t.Fatalf("Step %d: failed to serialize message: %v", i+1, err)
				}

				// Process the packet
				_, err = pp.ProcessPacket(string(packet), step.tRead)
				if err != nil {
					t.Fatalf("Step %d: failed to process packet: %v", i+1, err)
				}

				// Check expected messages for this step
				msgsAfterCount := len(handler.msgs)
				newMsgs := handler.msgs[msgsBeforeCount:msgsAfterCount]

				actualMsgTypes := 0
				for _, m := range newMsgs {
					switch m.msgType {
					case "time":
						actualMsgTypes |= testExpectTime
					case "satellites":
						actualMsgTypes |= testExpectSatellites
					case "leapsecond":
						actualMsgTypes |= testExpectLeapSecond
					case "survey":
						actualMsgTypes |= testExpectSurvey
					case "navepoch":
						actualMsgTypes |= testExpectNavEpoch
					}
				}

				if actualMsgTypes != step.expectMsgs {
					t.Errorf("Step %d (msgID=%v, epoch=%d): expected message types %b, got %b",
						i+1, step.msgID, step.navEpoch, step.expectMsgs, actualMsgTypes)
				}
			}

			// Build expected satellite message tReads
			expectedSatTReads := []time.Time{}
			var epoch uint32
			for _, step := range tt.steps {
				if (step.msgID == ubxbin.NavSatID || step.msgID == ubxbin.NavSigID) && step.navEpoch != epoch {
					expectedSatTReads = append(expectedSatTReads, step.tRead)
					epoch = step.navEpoch
				}
			}

			// Verify satellite message tReads
			satIndex := 0
			for _, m := range handler.msgs {
				if m.msgType == "satellites" {
					if satIndex >= len(expectedSatTReads) {
						t.Errorf("More satellite messages than expected")
						break
					}
					expectedTRead := expectedSatTReads[satIndex]
					if m.tRead != expectedTRead {
						t.Errorf("Satellite message %d: expected tRead %v, got %v",
							satIndex, expectedTRead, m.tRead)
					}
					satIndex++
				}
			}
		})
	}
}

func TestNavEpochMsg(t *testing.T) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	handler := &testMsgHandler{}
	pp.SetMsgHandler(handler)

	// Send messages for epoch 1 - no NavEpochMsg should be emitted
	for _, msgID := range []ubxbin.MsgID{ubxbin.NavTimeGPSID, ubxbin.NavSatID, ubxbin.NavSigID} {
		msg := testCreateMessage(msgID, 100)
		packet, err := ubxbin.Serialize(msg)
		if err != nil {
			t.Fatalf("failed to serialize: %v", err)
		}
		if _, err := pp.ProcessPacket(string(packet), time.Unix(1, 0)); err != nil {
			t.Fatalf("failed to process: %v", err)
		}
	}
	for _, m := range handler.msgs {
		if m.msgType == "navepoch" {
			t.Fatal("NavEpochMsg emitted before first epoch boundary")
		}
	}

	// Send first message of epoch 2 - should trigger NavEpochMsg for epoch 1
	msg := testCreateMessage(ubxbin.NavTimeGPSID, 200)
	packet, err := ubxbin.Serialize(msg)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	beforeCount := len(handler.msgs)
	if _, err := pp.ProcessPacket(string(packet), time.Unix(2, 0)); err != nil {
		t.Fatalf("failed to process: %v", err)
	}
	var epochMsg *gpsprot.NavEpochMsg
	var epochTRead time.Time
	for _, m := range handler.msgs[beforeCount:] {
		if m.msgType == "navepoch" {
			epochMsg = m.msg.(*gpsprot.NavEpochMsg)
			epochTRead = m.tRead
		}
	}
	if epochMsg == nil {
		t.Fatal("no NavEpochMsg emitted at epoch boundary")
	}
	if epochMsg.Tag != Tag {
		t.Fatalf("NavEpochMsg.Tag = %q, want %q", epochMsg.Tag, Tag)
	}
	if epochMsg.StartTime != time.Unix(1, 0) {
		t.Fatalf("NavEpochMsg.StartTime = %v, want %v", epochMsg.StartTime, time.Unix(1, 0))
	}
	if epochTRead != time.Unix(2, 0) {
		t.Fatalf("NavEpochMsg tRead = %v, want %v", epochTRead, time.Unix(2, 0))
	}
}

func testCreateMessage(msgID ubxbin.MsgID, navEpoch uint32) ubxbin.Msg {
	switch msgID {
	case ubxbin.NavSatID:
		msg := &ubxbin.NavSat{}
		msg.NavSatFixed.ITOW = navEpoch
		msg.Version = 1 // Required version
		// Add minimal satellite data
		msg.NumSVs = 1
		msg.SVs = []ubxbin.NavSatSV{{
			GNSSID: 0, // GPS
			SVID:   1,
			CNO:    40,
			Elev:   45,
			Azim:   90,
			PRRes:  100,
			Flags:  0x13, // Used, healthy
		}}
		return msg
	case ubxbin.NavSigID:
		msg := &ubxbin.NavSig{}
		msg.NavSigFixed.ITOW = navEpoch
		msg.Version = 0 // Version 0 for NavSig
		// Add minimal signal data
		msg.NumSigs = 1
		msg.Signals = []ubxbin.NavSigSignal{{
			GNSSID:     0, // GPS
			SVID:       1,
			SigID:      0, // L1 C/A
			FreqID:     0,
			PRRes:      100,
			CNO:        40,
			QualityInd: 7, // Good
			CorrSource: 0,
			IonoModel:  0,
			SigFlags:   0x13, // Health OK, tracking
		}}
		return msg
	case ubxbin.NavTimeGPSID:
		msg := &ubxbin.NavTimeGPS{}
		msg.ITOW = navEpoch
		msg.FTOW = 0
		msg.Week = 2250 // Example GPS week
		msg.LeapS = 18
		msg.Valid = 0x07 // Time, week, leap second valid
		msg.TAcc = 20    // 20ns accuracy
		return msg
	case ubxbin.NavEOEID:
		msg := &ubxbin.NavEOE{}
		msg.ITOW = navEpoch
		return msg
	default:
		panic("unsupported message ID for test")
	}
}
