package gpscmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
	"github.com/jclark/satpulse/internal/ubxcfgval"
)

var ubxSchema = ubxcfgval.NewSchemaWithMsgout(ubxcfgval.GetDfltSchema())

func TestReplay(t *testing.T) {
	for _, filename := range replayFiles {
		t.Run(filename, func(t *testing.T) {
			testReplayFile(t, filename)
		})
	}
}

func HideTestReplayBug(t *testing.T) {
	testReplayFile(t, "bug")
}

func testReplayFile(t *testing.T, name string) {
	path := filepath.Join("testdata", name+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	testNum := 0

	for {
		test, err := readTest(scanner)
		if err != nil {
			t.Fatal(err)
		}
		if test == nil {
			break
		}

		t.Run(fmt.Sprintf("%s_%d", name, testNum), func(t *testing.T) {
			r, err := newReplayer(t, test)
			if err != nil {
				t.Fatal(err)
			}
			r.run()
			r.verify()
		})
		testNum++
	}
}

type replayTest struct {
	env        TestLogEnvEntry
	inPackets  []gpsio.PacketLogEntry
	outPackets []gpsio.PacketLogEntry
	inBefore   []int // number of input packets before each output packet
	receiver   *TestLogReceiverEntry
	config     TestLogConfigEntry
}

func readTest(scanner *bufio.Scanner) (*replayTest, error) {
	// First line must be env
	if !scanner.Scan() {
		return nil, nil
	}

	test := &replayTest{}
	if err := json.Unmarshal(scanner.Bytes(), &test.env); err != nil {
		return nil, err
	}
	if test.env.Type != "env" {
		return nil, errors.New("expected env entry")
	}

	// Read packets and other entries until config
	for scanner.Scan() {
		// Try packet first
		// Note: json.Unmarshal succeeds on any valid JSON, so we need to check
		// if it actually parsed packet fields (like T) to know it's really a packet
		var pkt gpsio.PacketLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &pkt); err == nil && !time.Time(pkt.T).IsZero() {
			if pkt.Out {
				test.outPackets = append(test.outPackets, pkt)
				test.inBefore = append(test.inBefore, len(test.inPackets))
			} else {
				test.inPackets = append(test.inPackets, pkt)
			}
			continue
		}

		// Check for receiver or config
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &typed); err != nil {
			return nil, err
		}

		switch typed.Type {
		case "receiver":
			test.receiver = &TestLogReceiverEntry{}
			if err := json.Unmarshal(scanner.Bytes(), test.receiver); err != nil {
				return nil, err
			}
		case "config":
			if err := json.Unmarshal(scanner.Bytes(), &test.config); err != nil {
				return nil, err
			}
			return test, nil
		}
	}

	return nil, errors.New("test missing config entry")
}

type replayer struct {
	gpsprot.DefaultHandler
	t           *testing.T
	test        *replayTest
	target      *gpsprot.ConfigTarget
	packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor
	inIdx       int
	outIdx      int
	px          gpsprot.PacketExchanger
	cfgtor      gpsprot.Configurator
	configErr   error // first configuration error encountered
}

func newReplayer(t *testing.T, test *replayTest) (*replayer, error) {
	v, _, err := parseFlags("gps", test.env.Args)
	if err != nil {
		return nil, err
	}

	target, err := createConfigTarget(v)
	if err != nil {
		return nil, err
	}

	// Create packet processors like gpscfg does
	packetProcs := gpsreg.CreatePacketProcessors(nil)

	r := replayer{
		t:           t,
		test:        test,
		target:      target,
		packetProcs: packetProcs,
	}
	return &r, nil
}

func (r *replayer) run() {
	// Initialize packet processors like gpscfg does
	for _, pp := range r.packetProcs {
		pp.SetMsgHandler(r)
		if r.px == nil {
			r.px = pp.CreatePacketExchanger()
		}
	}

	// Feed input packets before first output packet (probe)
	r.feedUpTo(0)

	if len(r.test.outPackets) == 0 {
		// we decided not to probe, so nothing to do
		return
	}
	// Check the probe packet matches the first output packet
	probePacket := r.px.ProbePacket()

	expected := r.test.outPackets[0]
	if string(probePacket) != expected.Data() {
		r.t.Errorf("probe packet mismatch")
	}
	r.outIdx++

	// Feed input packets before second output packet
	r.feedUpTo(1)

	// Should be probed now
	if !r.px.ProbeOK() {
		r.t.Error("probe did not succeed after feeding initial packets")
		return
	}

	// Create configurator
	var err error
	r.cfgtor, err = r.px.Configure(r.target)
	if err != nil {
		r.t.Fatal(err)
	}

	// Process configuration requests
	for {
		req, err := r.cfgtor.NextRequest()
		if err != nil {
			r.t.Logf("NextRequest error: %v", err)
			if r.configErr == nil {
				r.configErr = err
			}
			continue
		}
		if req == nil {
			break
		}

		// Verify output packet matches
		if r.outIdx >= len(r.test.outPackets) {
			r.t.Fatalf("unexpected request %s, no more output packets", req.ID())
		}

		expected := r.test.outPackets[r.outIdx]
		actual := req.Packet()
		if !r.packetsEqual(req.ID(), actual, expected) {
			r.t.Errorf("packet mismatch for %s", req.ID())
		}
		r.outIdx++
		r.waitAfterSend(req, time.Time(expected.T), actual)

	}

	if r.outIdx < len(r.test.outPackets) {
		r.t.Fatalf("failed to generate all output packets, got %d, want %d",
			r.outIdx, len(r.test.outPackets))
	}
}

const maxTries = 3

func (r *replayer) waitAfterSend(req gpsprot.ConfigRequest, tSent time.Time, actual []byte) {
	var err error
	awaitingAck := req.Ackable()
	awaitingResp := true
	// tChecksumOK tracks the last time we received a packet with valid checksum
	var tChecksumOK time.Time

	for range maxTries {
		// Track packets with valid checksums before feeding them
		initialInIdx := r.inIdx

		// Feed all input packets before next output packet
		r.feedUpTo(r.outIdx)

		// Check for valid checksum packets that were just fed
		for i := initialInIdx; i < r.inIdx; i++ {
			p := r.test.inPackets[i]
			if p.Tag != "" { // packet has valid checksum if it has a tag
				tChecksumOK = time.Time(p.T)
			}
		}

		if awaitingAck {
			ack := r.cfgtor.FindAck(actual, tSent)
			if ack != nil {
				if !ack.OK {
					if r.configErr == nil {
						r.configErr = fmt.Errorf("NACK received for %s", req.ID())
					}
					return
				}
				req.Done()
				awaitingAck = false
			}
		}
		if awaitingResp {
			awaitingResp = req.AwaitingResponse(tSent)
		}
		if !awaitingAck && !awaitingResp {
			return
		}
		if awaitingAck {
			err = fmt.Errorf("no ACK received for %s", req.ID())
		} else {
			err = fmt.Errorf("no response received for %s", req.ID())
		}
		if r.outIdx < len(r.test.outPackets) && r.test.outPackets[r.outIdx].Data() == r.test.outPackets[r.outIdx-1].Data() {
			// we didn't get an ACK or response, and the next recorded output packet was the same as the last one;
			// this means (almost certainly) that originally there was a retry, so we can just skip over this and keep trying
			r.outIdx++
			continue
		}
		// do not retry
		break
	}

	// Handle speed change case: if no ACK but valid packets received after speed change sent
	if awaitingAck && req.ChangeSpeed() != 0 && tChecksumOK.After(tSent) {
		req.Done()
		return
	}

	if r.configErr == nil {
		r.configErr = err
	}
	r.cfgtor.Abort()
}

func (r *replayer) feedUpTo(outIdx int) {
	// Determine how many input packets to feed
	targetInIdx := len(r.test.inPackets)
	if outIdx < len(r.test.inBefore) {
		targetInIdx = r.test.inBefore[outIdx]
	}

	// Feed input packets
	for r.inIdx < targetInIdx {
		p := r.test.inPackets[r.inIdx]
		r.inIdx++

		// Skip packets without a tag (invalid packets)
		if p.Tag == "" {
			continue
		}

		// Get the packet processor for this packet's tag
		pp, ok := r.packetProcs[p.Tag]
		if !ok {
			r.t.Errorf("no processor for tag %s", p.Tag)
			continue
		}

		_, err := pp.ProcessPacket(p.Data(), time.Time(p.T))
		if err != nil {
			r.t.Errorf("error processing packet: %v", err)
		}
	}
}

func (r *replayer) packetsEqual(msgID string, actual []byte, expected gpsio.PacketLogEntry) bool {
	actualStr := string(actual)
	expectedStr := expected.Data()

	// First check if they're exactly equal
	if actualStr == expectedStr {
		return true
	}

	// If not, check special cases for messages that might have reordered data
	switch msgID {
	case "CFG-VALSET":
		return valsetPacketsEqual(r.t, actualStr, expectedStr)
	case "CFG-VALGET":
		// For output packets (which these always are), cfgData contains keys
		return valgetPacketsEqual(r.t, actualStr, expectedStr)
	default:
		// Try to parse both messages to provide better error details
		actualMsg, actualErr := ubxbin.ParseMsg(actualStr)
		expectedMsg, expectedErr := ubxbin.ParseMsg(expectedStr)

		if actualErr != nil {
			r.t.Errorf("%s: failed to parse actual packet: %v", msgID, actualErr)
		}
		if expectedErr != nil {
			r.t.Errorf("%s: failed to parse expected packet: %v", msgID, expectedErr)
		}

		if actualErr == nil && expectedErr == nil {
			r.t.Errorf("%s: packets differ: actual %+v, expected %+v", msgID, actualMsg, expectedMsg)
		} else if actualErr == nil || expectedErr == nil {
			r.t.Errorf("%s: packet content differs (length actual=%d, expected=%d)", msgID, len(actualStr), len(expectedStr))
		}

		return false
	}
}

func (r *replayer) verify() {
	props := r.cfgtor.ConfigProps()
	cfg := &r.test.config

	if cfg.Error != "" {
		if r.configErr != nil {
			r.t.Logf("error while logging %q; error while testing %q", cfg.Error, r.configErr.Error())
			return
		}
		r.t.Fatalf("error while logging %q; but no error while testing", cfg.Error)
	}
	if r.configErr != nil {
		r.t.Fatalf("no error while logging; but error while testing %q", r.configErr.Error())
	}

	// Verify signals
	if cfg.SignalsEnabled != nil {
		actual, ok := props.GetSignalsEnabled()
		if !ok {
			r.t.Error("signals not set")
		} else {
			actualGroups := actual.GNSSStringGroups()
			if !equalStringSlices(actualGroups, cfg.SignalsEnabled) {
				r.t.Errorf("signals mismatch: got %v, want %v", actualGroups, cfg.SignalsEnabled)
			}
		}
	}
	// Verify mode
	if cfg.Static != nil {
		mode, ok := props.GetMode()
		if !ok {
			r.t.Error("mode not set")
		} else {
			if mode.Static != *cfg.Static {
				r.t.Errorf("static mode mismatch: got %v, want %v", mode.Static, *cfg.Static)
			}
			if mode.PosType == gpsprot.PosTypeECEF && len(cfg.FixedPosECEF) == 3 {
				for i := range 3 {
					if mode.FixedPosECEF[i].Meters() != cfg.FixedPosECEF[i] {
						r.t.Errorf("fixed position ECEF mismatch at index %d: got %f, want %f",
							i, mode.FixedPosECEF[i].Meters(), cfg.FixedPosECEF[i])
					}
				}
				if mode.FixedPosAcc.Meters() != *cfg.FixedPosAcc {
					r.t.Errorf("fixed position accuracy mismatch: got %f, want %f",
						mode.FixedPosAcc.Meters(), *cfg.FixedPosAcc)
				}
			}
		}
	}

}

func equalStringSlices(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

func valsetPacketsEqual(t *testing.T, actual, expected string) bool {
	// Parse both packets
	actualMsg, err := ubxbin.ParseMsg(actual)
	if err != nil {
		t.Errorf("CFG-VALSET: failed to parse actual packet: %v", err)
		return false
	}
	expectedMsg, err := ubxbin.ParseMsg(expected)
	if err != nil {
		t.Errorf("CFG-VALSET: failed to parse expected packet: %v", err)
		return false
	}

	actualValset, ok := actualMsg.(*ubxbin.CfgValset)
	if !ok {
		return false
	}
	expectedValset, ok := expectedMsg.(*ubxbin.CfgValset)
	if !ok {
		t.Errorf("expected %s, but got CFG-VALSET", expectedMsg.ID().String())
		return false
	}

	// Compare fixed fields
	if actualValset.CfgValsetFixed != expectedValset.CfgValsetFixed {
		t.Errorf("CFG-VALSET: fixed fields differ: actual %+v, expected %+v",
			actualValset.CfgValsetFixed, expectedValset.CfgValsetFixed)
		return false
	}

	// Parse and compare configuration items
	actualKeys, actualValues, err := ubxSchema.UnmarshalItemsFlat(actualValset.CfgData)
	if err != nil {
		t.Errorf("CFG-VALSET: failed to unmarshal actual items: %v", err)
		return false
	}

	expectedKeys, expectedValues, err := ubxSchema.UnmarshalItemsFlat(expectedValset.CfgData)
	if err != nil {
		t.Errorf("CFG-VALSET: failed to unmarshal expected items: %v", err)
		return false
	}

	// Create maps for comparison
	actualMap := make(map[string]any)
	expectedMap := make(map[string]any)

	for i, key := range actualKeys {
		actualMap[key] = actualValues[i]
	}
	for i, key := range expectedKeys {
		expectedMap[key] = expectedValues[i]
	}

	// Check each expected key
	equal := true
	for key, expectedVal := range expectedMap {
		if actualVal, exists := actualMap[key]; !exists {
			t.Errorf("CFG-VALSET: missing key: %s (expected value: %v)", key, expectedVal)
			equal = false
		} else if actualVal != expectedVal {
			t.Errorf("CFG-VALSET: key %s: actual %v, expected %v", key, actualVal, expectedVal)
			equal = false
		}
	}

	// Check for unexpected keys
	for key, actualVal := range actualMap {
		if _, exists := expectedMap[key]; !exists {
			t.Errorf("CFG-VALSET: unexpected key: %s (actual value: %v)", key, actualVal)
			equal = false
		}
	}

	return equal
}

func valgetPacketsEqual(t *testing.T, actual, expected string) bool {
	// Parse both packets
	actualMsg, err := ubxbin.ParseMsg(actual)
	if err != nil {
		t.Errorf("CFG-VALGET: failed to parse actual packet: %v", err)
		return false
	}
	expectedMsg, err := ubxbin.ParseMsg(expected)
	if err != nil {
		t.Errorf("CFG-VALGET: failed to parse expected packet: %v", err)
		return false
	}

	actualValget, ok := actualMsg.(*ubxbin.CfgValget)
	if !ok {
		return false
	}
	expectedValget, ok := expectedMsg.(*ubxbin.CfgValget)
	if !ok {
		t.Errorf("expected %s, but got CFG-VALGET", expectedMsg.ID().String())
		return false
	}

	// Compare fixed fields
	if actualValget.CfgValgetFixed != expectedValget.CfgValgetFixed {
		t.Errorf("CFG-VALGET: fixed fields differ: actual %+v, expected %+v",
			actualValget.CfgValgetFixed, expectedValget.CfgValgetFixed)
		return false
	}

	// For output packets, cfgData contains keys only
	actualKeys, err := ubxSchema.UnmarshalKeysFlat(actualValget.CfgData)
	if err != nil {
		t.Errorf("CFG-VALGET: failed to unmarshal actual keys: %v", err)
		return false
	}

	expectedKeys, err := ubxSchema.UnmarshalKeysFlat(expectedValget.CfgData)
	if err != nil {
		t.Errorf("CFG-VALGET: failed to unmarshal expected keys: %v", err)
		return false
	}

	// Create maps for comparison (to ignore order)
	actualMap := make(map[string]bool)
	expectedMap := make(map[string]bool)

	for _, key := range actualKeys {
		actualMap[key] = true
	}
	for _, key := range expectedKeys {
		expectedMap[key] = true
	}

	// Check each expected key
	equal := true
	for key := range expectedMap {
		if !actualMap[key] {
			t.Errorf("CFG-VALGET: missing expected key: %s", key)
			equal = false
		}
	}

	// Check for unexpected keys
	for key := range actualMap {
		if !expectedMap[key] {
			t.Errorf("CFG-VALGET: unexpected extra key: %s", key)
			equal = false
		}
	}

	return equal
}
