package gpscmd

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

var update = flag.Bool("update", false, "update golden test data files")

// packetCmpFunc compares actual and expected packets for a specific protocol.
// Returns (equal, updatable): equal means packets match;
// updatable means the mismatch is safe to auto-update in golden files.
type packetCmpFunc func(t *testing.T, msgID string, actual []byte, expected gpsio.PacketLogEntry) (bool, bool)

func testReplayFile(t *testing.T, name string, packetCmp packetCmpFunc) {
	path := filepath.Join("testdata", name+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	testNum := 0
	globalInIdx := 0
	globalOutIdx := 0
	var replayers []*replayer

	for {
		test, err := readTest(scanner)
		if err != nil {
			t.Fatal(err)
		}
		if test == nil {
			break
		}

		outOffset := globalOutIdx
		inOffset := globalInIdx
		globalInIdx += len(test.inPackets)
		globalOutIdx += len(test.outPackets)

		t.Run(fmt.Sprintf("%s_%d", name, testNum), func(t *testing.T) {
			r, err := newReplayer(t, test, packetCmp)
			if err != nil {
				t.Fatal(err)
			}
			// Preserve any updatable packet diffs even if verify() fails later.
			defer func() {
				replayers = append(replayers, r)
			}()
			r.inOffset = inOffset
			r.outOffset = outOffset
			r.run()
			r.verify()
		})
		testNum++
	}

	if !*update {
		return
	}
	// Collect updates from non-structural replayers
	allUpdates := make(map[int][]byte)
	allInputUpdates := make(map[int][]byte)
	for _, r := range replayers {
		if r.structural {
			continue
		}
		for idx, pkt := range r.updates {
			allUpdates[idx] = pkt
		}
		for idx, pkt := range r.inputUpdates {
			allInputUpdates[idx] = pkt
		}
	}
	if len(allUpdates) == 0 && len(allInputUpdates) == 0 {
		return
	}
	if len(allUpdates) != 0 {
		applyUpdates(t, path, allUpdates, true)
	}
	if len(allInputUpdates) != 0 {
		applyUpdates(t, path, allInputUpdates, false)
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
			// Fix up inBefore to account for out-of-order packets
			fixupInBefore(test)
			return test, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, errors.New("test missing config entry")
}

// fixupInBefore adjusts the inBefore counts to deal with occasional relative
// out of order between input and output packets
func fixupInBefore(test *replayTest) {
	for i, outPkt := range test.outPackets {
		outTime := time.Time(outPkt.T)
		count := test.inBefore[i]

		// Check if we need to adjust by looking at packets around the boundary
		// Decrease count while the packet at count-1 has timestamp after outTime
		for count > 0 && time.Time(test.inPackets[count-1].T).After(outTime) {
			count--
		}
		// Increase count while the packet at count has timestamp before outTime
		for count < len(test.inPackets) && time.Time(test.inPackets[count].T).Before(outTime) {
			count++
		}

		test.inBefore[i] = count
	}
}

type replayer struct {
	gpsprot.DefaultHandler
	t             *testing.T
	test          *replayTest
	target        *gpsprot.ConfigTarget
	packetProcs   map[gpsprot.Tag]gpsprot.PacketProcessor
	configProts   []gpsprot.ConfigProtocol
	inIdx         int
	outIdx        int
	cp            gpsprot.ConfigProtocol
	cfgtor        gpsprot.Configurator
	director      *gpsprot.ConfigDirector // Store director for ValidPacketReceived calls
	configErr     error                   // first configuration error encountered
	packetCmp     packetCmpFunc
	replayTime    time.Time      // simulated current time for replay
	timeline      []time.Time    // sorted list of all packet timestamps
	timelineIdx   int            // current position in timeline
	inOffset      int            // offset of this block's input packets in the global file
	outOffset     int            // offset of this block's output packets in the global file
	updates       map[int][]byte // global output packet index -> new packet bytes
	inputUpdates  map[int][]byte // global input packet index -> new packet bytes
	pendingValget []valgetResponsePatch
	structural    bool // set on non-updatable failure
}

type valgetResponsePatch struct {
	keys []ucv.Key
}

func newReplayer(t *testing.T, test *replayTest, comparePackets packetCmpFunc) (*replayer, error) {
	v, _, err := parseFlags("gps", test.env.Args)
	if err != nil {
		return nil, err
	}

	target, err := createConfigTarget(v)
	if err != nil {
		return nil, err
	}

	// Create packet processors like gpscfg does
	packetProcs := gpsreg.CreatePacketProcessors(v.vendor)
	configProts := gpsreg.CreateConfigProtocols(v.vendor)

	// Build timeline of all packet timestamps
	timeline := make([]time.Time, 0, len(test.inPackets)+len(test.outPackets))
	seen := make(map[time.Time]bool)

	for _, p := range test.inPackets {
		t := time.Time(p.T)
		if !seen[t] {
			timeline = append(timeline, t)
			seen[t] = true
		}
	}
	for _, p := range test.outPackets {
		t := time.Time(p.T)
		if !seen[t] {
			timeline = append(timeline, t)
			seen[t] = true
		}
	}

	// Sort the timeline
	slices.SortFunc(timeline, func(a, b time.Time) int {
		if a.Before(b) {
			return -1
		}
		if a.After(b) {
			return 1
		}
		return 0
	})

	var replayTime time.Time
	if len(timeline) > 0 {
		replayTime = timeline[0]
	} else {
		replayTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	r := replayer{
		t:           t,
		test:        test,
		target:      target,
		packetProcs: packetProcs,
		configProts: configProts,
		packetCmp:   comparePackets,
		replayTime:  replayTime,
		timeline:    timeline,
		timelineIdx: 0,
	}
	return &r, nil
}

func (r *replayer) run() {
	// Initialize packet processors like gpscfg does
	gpsprot.SetAllMsgHandlers(r.packetProcs, r)

	// Set up MultiNativeMsgHandler to fan out to all protocols during probing
	handlers := make([]gpsprot.NativeMsgHandler, len(r.configProts))
	for i, prot := range r.configProts {
		handlers[i] = prot
	}
	mnmh := gpsprot.NewMultiNativeMsgHandler(handlers...)
	for _, pp := range r.packetProcs {
		pp.SetNativeMsgHandler(mnmh)
	}

	// Feed input packets before first output packet (probe)
	r.feedUpTo(0)

	if len(r.test.outPackets) == 0 {
		// we decided not to probe, so nothing to do
		return
	}

	// Send probe packets for all protocols
	probesSent := 0
	for _, prot := range r.configProts {
		probePacket := prot.ProbePacket()

		// Verify probe packet matches expected output
		if r.outIdx < len(r.test.outPackets) {
			expected := r.test.outPackets[r.outIdx]
			if string(probePacket) == expected.Data() {
				r.outIdx++
				probesSent++
			}
		}
	}

	if probesSent == 0 {
		r.t.Error("no matching probe packets found")
		r.structural = true
		return
	}

	// Feed input packets after probe packets
	r.feedUpTo(r.outIdx)

	// Check which protocol succeeded
	for _, prot := range r.configProts {
		if prot.ProbeOK() {
			r.cp = prot
			// Switch to single protocol for configuration
			mnmh.Reset(prot)
			break
		}
	}

	if r.cp == nil {
		r.t.Error("probe did not succeed after feeding initial packets")
		r.structural = true
		return
	}

	// Create configurator
	var err error
	r.cfgtor, err = r.cp.Configure(r.target)
	if err != nil {
		r.t.Fatal(err)
	}

	director := gpsprot.NewConfigDirector(r.cfgtor, maxTries)
	r.director = director // Store for ValidPacketReceived calls

	// Process configuration using director
	for action := range director.Actions() {
		// Advance director's time to match replay time for automatic timeout processing
		director.AdvanceTimeTo(r.replayTime)

		switch action.Type {
		case gpsprot.ConfigActionSendRequest:
			// Verify output packet matches expected
			if r.outIdx >= len(r.test.outPackets) {
				r.t.Fatalf("unexpected request, no more output packets")
			}

			expected := r.test.outPackets[r.outIdx]
			// Extract message ID from packet for proper comparison
			msgID := ""
			if msg, err := ubxbin.ParseMsg(string(action.Packet)); err == nil {
				msgID = msg.ID().String()
			}
			eq, updatable := r.packetCmp(r.t, msgID, action.Packet, expected)
			if *update {
				if keys := cfgValgetRequestKeys(action.Packet); len(keys) != 0 {
					r.pendingValget = append(r.pendingValget, valgetResponsePatch{keys: keys})
				}
			}
			if !eq {
				r.t.Errorf("packet mismatch")
				if updatable && *update {
					if r.updates == nil {
						r.updates = make(map[int][]byte)
					}
					r.updates[r.outOffset+r.outIdx] = slices.Clone(action.Packet)
				} else if !updatable {
					r.structural = true
					return
				}
			}
			r.outIdx++

			// Advance time to the send time, processing any input packets with earlier timestamps
			sendTime := time.Time(expected.T)
			r.advanceTimeTo(sendTime)

			// Now mark as sent using recorded timestamp
			req := r.cfgtor.Request(action.Index)
			req.SetSentTime(sendTime)

		case gpsprot.ConfigActionWaitUntil:
			// The director may batch more SendRequest actions after observing an
			// earlier pending deadline. By the time it yields WaitUntil, replayTime
			// can already be past that deadline, which means the wait is already
			// satisfied and must not rewind simulated time.
			if !action.Deadline.After(r.replayTime) {
				continue
			}

			// Advance to the next timeline instant, but not past the deadline
			// This allows the director to re-evaluate after each packet is processed
			if !r.advanceToNextInstant(action.Deadline) {
				// No packets found before deadline, advance to deadline
				r.replayTime = action.Deadline
			}

		case gpsprot.ConfigActionError:
			if r.configErr == nil {
				r.configErr = action.Error
			}
			// Continue to allow recovery operations
		}
	}

	if r.outIdx < len(r.test.outPackets) {
		r.structural = true
		r.t.Fatalf("failed to generate all output packets, got %d, want %d",
			r.outIdx, len(r.test.outPackets))
	}
}

const maxTries = 3

// advanceToNextInstant advances to the next timeline instant, but not past the deadline
// Returns true if we advanced to a new instant, false if we've reached the deadline or end of timeline
func (r *replayer) advanceToNextInstant(deadline time.Time) bool {
	if r.timelineIdx >= len(r.timeline) {
		// No more timeline instants
		if deadline.After(r.replayTime) {
			r.replayTime = deadline
		}
		return false
	}

	instant := r.timeline[r.timelineIdx]
	if instant.After(deadline) {
		// Next instant is past the deadline
		if deadline.After(r.replayTime) {
			r.replayTime = deadline
		}
		return false
	}

	// Feed any input packets at this instant
	for r.inIdx < len(r.test.inPackets) {
		p := r.test.inPackets[r.inIdx]
		pktTime := time.Time(p.T)

		if pktTime.After(instant) {
			break // This packet is for a later time
		}

		if pktTime.Equal(instant) {
			r.inIdx++
			if p.Tag == "" {
				continue // Skip invalid packets
			}

			data := r.maybePatchInputPacket(r.inIdx-1, p)
			pp, ok := r.packetProcs[p.Tag]
			if !ok {
				r.t.Errorf("no processor for tag %s", p.Tag)
				continue
			}

			_, err := pp.ProcessPacket(data, pktTime)
			if err != nil {
				r.t.Errorf("error processing packet at %v: %v (data: %q)", pktTime, err, data)
			} else if r.director != nil {
				// Notify director of valid packet for speed change handling
				r.director.ValidPacketReceived(pktTime)
			}
		} else {
			// This packet is before the current instant, should have been processed already
			r.inIdx++
		}
	}

	r.replayTime = instant
	r.timelineIdx++
	return true
}

// advanceTimeTo advances simulated time to the target, feeding input packets along the way
func (r *replayer) advanceTimeTo(target time.Time) {
	for r.advanceToNextInstant(target) {
		// Keep advancing until we reach the target or run out of instants
	}
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

		pktTime := time.Time(p.T)
		r.replayTime = pktTime

		data := r.maybePatchInputPacket(r.inIdx-1, p)

		// Get the packet processor for this packet's tag
		pp, ok := r.packetProcs[p.Tag]
		if !ok {
			r.t.Errorf("no processor for tag %s", p.Tag)
			continue
		}

		_, err := pp.ProcessPacket(data, pktTime)
		if err != nil {
			r.t.Errorf("error processing packet at %v: %v (data: %q)", pktTime, err, data)
		}
	}
}

func (r *replayer) verify() {
	if r.structural || r.cfgtor == nil {
		return
	}

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
			actualMap := actual.GNSSSignalMap()
			if !equalSignalMaps(actualMap, cfg.SignalsEnabled) {
				r.t.Errorf("signals mismatch: got %v, want %v", actualMap, cfg.SignalsEnabled)
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
	// Verify TimeGNSS
	if cfg.TimeGNSS != "" {
		timeGNSS, ok := props.GetTimeGNSS()
		if !ok {
			r.t.Error("timeGNSS not set")
		} else {
			if timeGNSS.String() != cfg.TimeGNSS {
				r.t.Errorf("timeGNSS mismatch: got %v, want %v", timeGNSS.String(), cfg.TimeGNSS)
			}
		}
	}
	// Verify AntennaCableDelay
	if cfg.AntennaCableDelay != nil {
		antCableDelay, ok := props.GetAntennaCableDelay()
		if !ok {
			r.t.Error("antennaCableDelay not set")
		} else {
			if antCableDelay.Nanoseconds() != *cfg.AntennaCableDelay {
				r.t.Errorf("antennaCableDelay mismatch: got %v, want %v", antCableDelay.Nanoseconds(), *cfg.AntennaCableDelay)
			}
		}
	}
	// Verify TimePulse
	if cfg.TimePulse != nil {
		timePulse, ok := props.GetTimePulse()
		if !ok {
			r.t.Error("timePulse not set")
		} else {
			// Check if enabled state matches
			if timePulse.Width == 0 && cfg.TimePulse.timePulseEnabled() {
				r.t.Error("timePulse config shows enabled but actual has zero width")
			} else if timePulse.Width > 0 && !cfg.TimePulse.timePulseEnabled() {
				r.t.Error("timePulse config shows disabled but actual has non-zero width")
			}

			// If both are enabled, compare detailed properties
			if cfg.TimePulse.Enabled && timePulse.Width != 0 {
				if cfg.TimePulse.Width != nil && timePulse.Width.Seconds() != *cfg.TimePulse.Width {
					r.t.Errorf("timePulse width mismatch: got %v, want %v", timePulse.Width.Seconds(), *cfg.TimePulse.Width)
				}
				if cfg.TimePulse.Period != nil && timePulse.Period.Seconds() != *cfg.TimePulse.Period {
					r.t.Errorf("timePulse period mismatch: got %v, want %v", timePulse.Period.Seconds(), *cfg.TimePulse.Period)
				}
				if cfg.TimePulse.PolarityRising != nil && timePulse.PolarityRising != *cfg.TimePulse.PolarityRising {
					r.t.Errorf("timePulse polarity mismatch: got %v, want %v", timePulse.PolarityRising, *cfg.TimePulse.PolarityRising)
				}
				if cfg.TimePulse.OnlyWhenLocked != nil && timePulse.OnlyWhenLocked != *cfg.TimePulse.OnlyWhenLocked {
					r.t.Errorf("timePulse onlyWhenLocked mismatch: got %v, want %v", timePulse.OnlyWhenLocked, *cfg.TimePulse.OnlyWhenLocked)
				}
			}
		}
	}
	if cfg.RTCMBaseID != nil {
		rtcmBaseID, ok := props.GetRTCMBaseID()
		if !ok {
			r.t.Error("rtcmBaseID not set")
		} else if rtcmBaseID != *cfg.RTCMBaseID {
			r.t.Errorf("rtcmBaseID mismatch: got %v, want %v", rtcmBaseID, *cfg.RTCMBaseID)
		}
	}
	if cfg.BaudRate != nil {
		baudRate, ok := props.GetBaudRate()
		if !ok {
			r.t.Error("baudRate not set")
		} else if baudRate != *cfg.BaudRate {
			r.t.Errorf("baudRate mismatch: got %v, want %v", baudRate, *cfg.BaudRate)
		}
	}
	if cfg.Port != "" {
		port, ok := props.GetPort()
		if !ok {
			r.t.Error("port not set")
		} else if port != cfg.Port {
			r.t.Errorf("port mismatch: got %q, want %q", port, cfg.Port)
		}
	}
}

func equalSignalMaps(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !slices.Equal(av, bv) {
			return false
		}
	}
	return true
}

var binFieldRe = regexp.MustCompile(`"bin":"[0-9a-f]*"`)

// applyUpdates rewrites a JSONL file, replacing the bin field of packet lines
// at the specified global indices with new hex-encoded packet data.
func applyUpdates(t *testing.T, path string, updates map[int][]byte, wantOut bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s for update: %v", path, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning %s: %v", path, err)
	}
	inIdx := 0
	outIdx := 0
	updated := 0
	for i, line := range lines {
		if !isPacketLine(line) {
			continue
		}
		if isOutPacketLine(line) {
			if wantOut {
				if newPkt, ok := updates[outIdx]; ok {
					newHex := hex.EncodeToString(newPkt)
					lines[i] = binFieldRe.ReplaceAllString(line, `"bin":"`+newHex+`"`)
					updated++
				}
			}
			outIdx++
			continue
		}
		if !wantOut {
			if newPkt, ok := updates[inIdx]; ok {
				newHex := hex.EncodeToString(newPkt)
				lines[i] = binFieldRe.ReplaceAllString(line, `"bin":"`+newHex+`"`)
				updated++
			}
		}
		inIdx++
	}
	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	kind := "input"
	if wantOut {
		kind = "output"
	}
	t.Logf("updated %s: %d %s packets changed", path, updated, kind)
}

// isOutPacketLine reports whether a JSONL line is an output packet entry.
func isOutPacketLine(line string) bool {
	// Output packets have "out":true and a "bin" field
	return bytes.Contains([]byte(line), []byte(`"out":true`))
}

func isPacketLine(line string) bool {
	b := []byte(line)
	if !bytes.Contains(b, []byte(`"t":"`)) {
		return false
	}
	return bytes.Contains(b, []byte(`"bin":"`)) || bytes.Contains(b, []byte(`"ascii":"`))
}

func cfgValgetRequestKeys(actualPacket []byte) []ucv.Key {
	actualMsg, err := ubxbin.ParseMsg(string(actualPacket))
	if err != nil {
		return nil
	}
	actualValget, ok := actualMsg.(*ubxbin.CfgValget)
	if !ok || actualValget.Version != ubxbin.CfgValgetVersionRequest {
		return nil
	}
	keys, err := ucv.UnmarshalKeys(actualValget.CfgData)
	if err != nil {
		return nil
	}
	return keys
}

func (r *replayer) maybePatchInputPacket(localIdx int, p gpsio.PacketLogEntry) string {
	if !*update || len(r.pendingValget) == 0 {
		return p.Data()
	}
	msg, err := ubxbin.ParseMsg(p.Data())
	if err != nil {
		return p.Data()
	}
	valget, ok := msg.(*ubxbin.CfgValget)
	if !ok || valget.Version != ubxbin.CfgValgetVersionResponse {
		return p.Data()
	}
	patch := r.pendingValget[0]
	r.pendingValget = r.pendingValget[1:]
	items, err := ucv.UnmarshalItems(valget.CfgData)
	if err != nil {
		return p.Data()
	}
	have := make(map[ucv.Key]struct{}, len(items))
	for _, item := range items {
		have[item.Key] = struct{}{}
	}
	changed := false
	for _, key := range patch.keys {
		if _, ok := have[key]; ok {
			continue
		}
		value, ok := synthesizeCfgValValue(key, &r.test.config)
		if !ok {
			continue
		}
		items = append(items, ucv.Item{Key: key, Value: value})
		changed = true
	}
	if !changed {
		return p.Data()
	}
	ucv.SortItems(items)
	cfgData, err := ucv.MarshalItems(items)
	if err != nil {
		return p.Data()
	}
	patched := *valget
	patched.CfgData = cfgData
	bytes, err := ubxbin.Serialize(&patched)
	if err != nil {
		return p.Data()
	}
	if r.inputUpdates == nil {
		r.inputUpdates = make(map[int][]byte)
	}
	r.inputUpdates[r.inOffset+localIdx] = slices.Clone(bytes)
	return string(bytes)
}

func synthesizeCfgValValue(key ucv.Key, cfg *TestLogConfigEntry) (uint64, bool) {
	switch key {
	case ucv.KRtcmDf003Out.Key():
		if cfg != nil && cfg.RTCMBaseID != nil {
			return uint64(*cfg.RTCMBaseID), true
		}
		return 0, true
	default:
		return 0, false
	}
}
