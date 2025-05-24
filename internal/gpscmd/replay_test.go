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
)

func TestReplayZ9PNoop(t *testing.T) {
	testReplayFile(t, "z9p-noop")
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
			runTest(t, test)
		})
		testNum++
	}
}

type replayTest struct {
	env      TestLogEnvEntry
	packets  []gpsio.PacketLogEntry
	receiver *TestLogReceiverEntry
	config   TestLogConfigEntry
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
		var pkt gpsio.PacketLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &pkt); err != nil {
			test.packets = append(test.packets, pkt)
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

func runTest(t *testing.T, test *replayTest) {
	v, _, err := parseFlags("gps", test.env.Args)
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("parseFlags returned nil")
	}

	target, err := createConfigTarget(v)
	if err != nil {
		t.Fatal(err)
	}

	// Create packet processors like gpscfg does
	packetProcs := gpsreg.CreatePacketProcessors(nil)

	r := &replayer{
		t:           t,
		test:        test,
		target:      target,
		packetProcs: packetProcs,
	}
	r.run()
}

type replayer struct {
	gpsprot.DefaultHandler
	t           *testing.T
	test        *replayTest
	target      *gpsprot.ConfigTarget
	packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor
	idx         int
	px          gpsprot.PacketExchanger
	cfgtor      gpsprot.Configurator
}

func (r *replayer) run() {
	// Initialize packet processors like gpscfg does
	for _, pp := range r.packetProcs {
		pp.SetMsgHandler(r)
		if r.px == nil {
			r.px = pp.CreatePacketExchanger()
		}
	}

	// Check the probe packet matches the first output packet
	probePacket := r.px.ProbePacket()
	if r.idx < len(r.test.packets) && r.test.packets[r.idx].Out {
		expected := r.test.packets[r.idx]
		if string(probePacket) != string(expected.Bin) {
			r.t.Errorf("probe packet mismatch")
		}
		r.idx++
	}

	// Feed packets until probe succeeds
	for !r.px.ProbeOK() && r.idx < len(r.test.packets) {
		r.feedNext()
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
			continue
		}
		if req == nil {
			break
		}
		r.processRequest(req)
	}

	// Feed remaining packets
	for r.idx < len(r.test.packets) {
		r.feedNext()
	}

	r.verify()
}

func (r *replayer) processRequest(req gpsprot.ConfigRequest) {
	// Find next output packet
	outIdx := r.idx
	for outIdx < len(r.test.packets) && !r.test.packets[outIdx].Out {
		outIdx++
	}

	if outIdx >= len(r.test.packets) {
		r.t.Fatalf("unexpected request %s, no more output packets", req.ID())
	}

	// Verify packet matches
	expected := r.test.packets[outIdx]
	actual := req.Packet()
	if string(actual) != string(expected.Bin) {
		r.t.Errorf("packet mismatch for %s", req.ID())
	}

	tSent := time.Time(expected.T)
	done := false
	acked := false

	// Feed packets up to and including this output packet
	for r.idx <= outIdx {
		r.feedNext()
	}

	// Process responses
	for r.idx < len(r.test.packets) {
		p := r.test.packets[r.idx]
		pTime := time.Time(p.T)
		if pTime.Sub(tSent) > 2*time.Second {
			break
		}

		r.feedNext()

		if req.Ackable() && !acked {
			ack := r.cfgtor.FindAck(actual, tSent)
			if ack != nil {
				if ack.OK {
					req.Done()
					done = true
				}
				acked = true
			}
		}

		if done || (!req.AwaitingResponse(tSent) && !req.Ackable()) {
			break
		}
	}
}

func (r *replayer) feedNext() {
	if r.idx >= len(r.test.packets) {
		return
	}
	p := r.test.packets[r.idx]
	r.idx++

	// Skip output packets - we only feed input packets
	if p.Out {
		return
	}

	// Skip packets without a tag (invalid packets)
	if p.Tag == "" {
		return
	}

	// Get the packet processor for this packet's tag
	pp, ok := r.packetProcs[p.Tag]
	if !ok {
		r.t.Errorf("no processor for tag %s", p.Tag)
		return
	}

	_, err := pp.ProcessPacket(string(p.Bin), time.Time(p.T))
	if err != nil {
		r.t.Errorf("error processing packet: %v", err)
	}
}

func (r *replayer) verify() {
	props := r.cfgtor.ConfigProps()
	cfg := &r.test.config

	if cfg.Error != "" {
		r.t.Logf("expected error %q not yet implemented", cfg.Error)
		return
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

	// Verify baud rate
	if cfg.BaudRate != nil {
		actual, ok := props.GetBaudRate()
		if !ok {
			r.t.Error("baud rate not set")
		} else if actual != *cfg.BaudRate {
			r.t.Errorf("baud rate mismatch: got %d, want %d", actual, *cfg.BaudRate)
		}
	}

	// Verify NMEA
	if cfg.NMEAEnabled != nil {
		actual, ok := props.GetNMEAEnabled()
		if !ok {
			r.t.Error("NMEA enabled not set")
		} else if actual != *cfg.NMEAEnabled {
			r.t.Errorf("NMEA enabled mismatch: got %v, want %v", actual, *cfg.NMEAEnabled)
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
