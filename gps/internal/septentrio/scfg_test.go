package septentrio

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// grcReplyPacket is a framed grc reply as captured from the mosaic-G5.
const grcReplyPacket = "$R: getReceiverCapabilities\r\n  " + grcStateLine + "\r\nUSB1>"

// receiverSetupPacket is a verbatim ReceiverSetup SBF packet from the
// committed mosaic-G5 corpus (gps/testdata/packets/septentrio/mosaic-G5).
const receiverSetupPacket = "244020a30e97a80140a9be1b79090000534550540000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000556e6b6e6f776e00000000000000000000000000556e6b6e6f776e00000000000000000000000000556e6b6e6f776e00000000000000000000000000000000000000000000000000000000000000000031303030313935373700000000000000000000004752423030363000000000000000000000000000312e312e30000000000000000000000000000000556e6b6e6f776e00000000000000000000000000556e6b6e6f776e00000000000000000000000000000000000000000000000000556e6b6e6f776e00000000000000000000000000323032362e30312e322d6738336431636136373730000000000000000000000000000000000000006d6f736169632d4735205033000000000000000000000000000000000000000000000000000000001874ef155eadce3f541c41aff51afc3faaabc2c1000000000000000000000000000000000000000000000000000000000000000000000000"

// TestConfigureRun drives a full configuration through the real
// ReplyProcessor / SBF PacketProcessor -> ConfigProtocol -> ConfigDirector
// path against a fake receiver that behaves as the mosaic-G5 was verified
// to: the probe reply carries the capability line and our prompt, the escape
// elicits no framed reply, and ReceiverSetup arrives as a normal SBF block
// on the block's own schedule.
func TestConfigureRun(t *testing.T) {
	cp := NewConfigProtocol()
	rp := NewReplyProcessor()
	rp.SetNativeMsgHandler(cp)
	sp := NewPacketProcessor(nil)
	sp.SetNativeMsgHandler(cp)
	t0 := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)

	if got, want := string(cp.ProbePacket()), "getReceiverCapabilities\r\n"; got != want {
		t.Fatalf("ProbePacket: got %q want %q", got, want)
	}
	if _, err := rp.ProcessPacket(grcReplyPacket, t0); err != nil {
		t.Fatalf("probe reply: %v", err)
	}
	if !cp.ProbeOK() {
		t.Fatalf("ProbeOK false after grc reply")
	}

	cfgIntf, err := cp.Configure(&gpsprot.ConfigTarget{})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	cfg := cfgIntf.(*Configurator)
	director := gpsprot.NewConfigDirector(cfg, 2)
	var sent []string
	rxSetupFed := false
	for action := range director.Actions() {
		switch action.Type {
		case gpsprot.ConfigActionSendRequest:
			t0 = t0.Add(time.Millisecond)
			sent = append(sent, strings.TrimSuffix(string(action.Packet), "\r\n"))
			cfg.Request(action.Index).SetSentTime(t0)
		case gpsprot.ConfigActionWaitUntil:
			t0 = action.Deadline
			if !rxSetupFed {
				// The periodic ReceiverSetup block arrives mid-run.
				rxSetupFed = true
				pkt, err := hex.DecodeString(receiverSetupPacket)
				if err != nil {
					t.Fatalf("decoding ReceiverSetup hex: %v", err)
				}
				if _, err := sp.ProcessPacket(string(pkt), t0); err != nil {
					t.Fatalf("ReceiverSetup packet: %v", err)
				}
			}
			director.AdvanceTimeTo(t0)
		case gpsprot.ConfigActionError:
			t.Fatalf("unexpected error action: %v", action.Error)
		}
	}
	if want := []string{escapeCmd}; !strings.HasPrefix(strings.Join(sent, "|"), strings.Join(want, "|")) {
		t.Errorf("sent packets: got %v want prefix %v", sent, want)
	}

	props := cfg.ConfigProps()
	if port, ok := props.GetPort(); !ok || port != "USB1" {
		t.Errorf("Port: got %q, %v; want USB1 from the probe reply prompt", port, ok)
	}
	if baud, ok := props.GetBaudRate(); !ok || baud != 0 {
		t.Errorf("BaudRate: got %d, %v; want 0 (not applicable on USB)", baud, ok)
	}

	info := cfg.ReceiverInfo()
	if info.Vendor != Vendor {
		t.Errorf("Vendor: got %q want %q", info.Vendor, Vendor)
	}
	if info.Hardware != "mosaic-G5 P3" {
		t.Errorf("Hardware: got %q want %q", info.Hardware, "mosaic-G5 P3")
	}
	if info.Firmware != "1.1.0" {
		t.Errorf("Firmware: got %q want %q", info.Firmware, "1.1.0")
	}
	if info.SupportedGNSS != gpsprot.AllGNSSSet {
		t.Errorf("SupportedGNSS: got %v want %v", info.SupportedGNSS, gpsprot.AllGNSSSet)
	}
}

// TestConfiguratorReply exercises the reply correlation rules directly: acks
// match the in-flight request by exact echo, refusals attribute to it by the
// single-flight discipline.
func TestConfiguratorReply(t *testing.T) {
	tests := []struct {
		name        string
		req         *sReq
		reply       *Reply
		expectState sReqState
		expectErr   string // substring of GetError, "" for none
	}{
		{
			name:        "ack with matching echo succeeds",
			req:         &sReq{state: sStateAwaiting, cmd: "setElevationMask, PVT, 10"},
			reply:       &Reply{Kind: ReplyAck, Echo: "setElevationMask, PVT, 10", States: []string{"ElevationMask, PVT, 10"}, Prompt: "USB1"},
			expectState: sStateSucceeded,
		},
		{
			name:        "ack with mismatched echo is ignored",
			req:         &sReq{state: sStateAwaiting, cmd: "setElevationMask, PVT, 10"},
			reply:       &Reply{Kind: ReplyAck, Echo: "getReceiverCapabilities", States: []string{grcStateLine}, Prompt: "USB1"},
			expectState: sStateAwaiting,
		},
		{
			name:        "nak fails the in-flight request with the receiver's text",
			req:         &sReq{state: sStateAwaiting, cmd: "setSignalTracking, +FOO"},
			reply:       &Reply{Kind: ReplyNak, Error: "setSignalTracking: Argument 'Signal' is invalid!", Prompt: "USB1"},
			expectState: sStateFailed,
			expectErr:   "Argument 'Signal' is invalid!",
		},
		{
			name:        "nak succeeds a nakOK request",
			req:         &sReq{state: sStateAwaiting, cmd: "setSignalTracking, +FOO", nakOK: true},
			reply:       &Reply{Kind: ReplyNak, Error: "setSignalTracking: Argument 'Signal' is invalid!", Prompt: "USB1"},
			expectState: sStateSucceeded,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Configurator{reqs: []*sReq{tc.req}}
			var got *Reply
			tc.req.onReply = func(r *Reply) { got = r }
			if err := c.reply(tc.reply, time.Time{}); err != nil {
				t.Fatalf("reply: %v", err)
			}
			if tc.req.state != tc.expectState {
				t.Errorf("state: got %v want %v", tc.req.state, tc.expectState)
			}
			if c.port != tc.reply.Prompt {
				t.Errorf("port: got %q want %q", c.port, tc.reply.Prompt)
			}
			if tc.expectErr != "" {
				if err := tc.req.GetError(); err == nil || !strings.Contains(err.Error(), tc.expectErr) {
					t.Errorf("GetError: got %v want substring %q", err, tc.expectErr)
				}
			}
			if tc.expectState == sStateSucceeded && got != tc.reply {
				t.Errorf("onReply not called with the matching reply")
			}
		})
	}
}

// TestPromote checks the single-flight rule: only the first non-final
// request is ever promoted to ReadyToSend.
func TestPromote(t *testing.T) {
	c := &Configurator{reqs: []*sReq{
		{state: sStateSucceeded, cmd: "a"},
		{state: sStateNotReady, cmd: "b"},
		{state: sStateNotReady, cmd: "c"},
	}}
	c.promote()
	if got := c.reqs[1].state; got != sStateReady {
		t.Errorf("reqs[1]: got %v want %v", got, sStateReady)
	}
	if got := c.reqs[2].state; got != sStateNotReady {
		t.Errorf("reqs[2]: got %v want %v", got, sStateNotReady)
	}
}
