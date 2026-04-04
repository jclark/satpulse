package gpscmd

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jclark/satpulse/gps/msgfile"
	"github.com/jclark/satpulse/gps/scan"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestFormatMsgID(t *testing.T) {
	tests := []struct {
		mid  msgfile.MsgID
		want string
	}{
		{msgfile.MsgID{Tag: "setup", Index: 0, Count: 1}, "setup"},
		{msgfile.MsgID{Tag: "setup", Index: 0, Count: 3}, "setup/1"},
		{msgfile.MsgID{Tag: "setup", Index: 2, Count: 3}, "setup/3"},
		{msgfile.MsgID{Tag: "", Index: 0, Count: 1}, ""},
		{msgfile.MsgID{Tag: "", Index: 1, Count: 3}, "/2"},
	}
	for _, tc := range tests {
		got := formatMsgID(tc.mid)
		if got != tc.want {
			t.Errorf("formatMsgID(%+v) = %q, want %q", tc.mid, got, tc.want)
		}
	}
}

func TestFormatAck(t *testing.T) {
	rm := msgfile.RawMsg{Tag: "cfg", Index: 2, Count: 5}
	tests := []struct {
		name string
		cor  msgfile.Correlation
		want string
	}{
		{"success", msgfile.Correlation{Ack: msgfile.AckAck, InResponseTo: &rm}, "cfg/3: OK\n"},
		{"NAK", msgfile.Correlation{Ack: msgfile.AckNak, InResponseTo: &rm}, "cfg/3: receiver rejected message: NAK\n"},
		{"NAK with error", msgfile.Correlation{Ack: msgfile.AckNak, NakError: "Invalid parameter", InResponseTo: &rm}, "cfg/3: receiver rejected message: Invalid parameter\n"},
		{"processing", msgfile.Correlation{Ack: msgfile.AckOther, InResponseTo: &rm}, "cfg/3: processing...\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatAck(tc.cor)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatText(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"strips CRLF", "$PTEST,data*00\r\n", "$PTEST,data*00\n"},
		{"strips LF only", "$PTEST,data*00\n", "$PTEST,data*00\n"},
		{"empty after strip", "\r\n", ""},
		{"non-printable skipped", "hello\x01world\r\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := scan.Packet{Data: tc.data}
			got := formatText(pkt)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatCorrelation(t *testing.T) {
	rh := &responseHandler{lg: testLogger(), cor: msgfile.NewCorrelator()}
	rm := msgfile.RawMsg{Tag: "valset", Index: 0, Count: 1}
	tests := []struct {
		name string
		cor  msgfile.Correlation
		pkt  scan.Packet
		want string
	}{
		{"not response", msgfile.Correlation{Relevance: msgfile.LevelNotResponse}, scan.Packet{}, ""},
		{"ack only", msgfile.Correlation{Ack: msgfile.AckAck, Relevance: msgfile.LevelAckOnly, InResponseTo: &rm}, scan.Packet{}, "valset: OK\n"},
		{"nak only", msgfile.Correlation{Ack: msgfile.AckNak, Relevance: msgfile.LevelAckOnly, InResponseTo: &rm}, scan.Packet{}, "valset: receiver rejected message: NAK\n"},
		{"maybe response", msgfile.Correlation{Relevance: msgfile.LevelMaybeResponse}, scan.Packet{Data: "$PTEST,hello*00\r\n"}, "$PTEST,hello*00\n"},
		{"sole response", msgfile.Correlation{Relevance: msgfile.LevelSoleResponse}, scan.Packet{Data: "$CONFIG,HEADING,0.0*xx\r\n"}, "$CONFIG,HEADING,0.0*xx\n"},
		{"ack + data", msgfile.Correlation{Ack: msgfile.AckAck, Relevance: msgfile.LevelSoleResponse, InResponseTo: &rm}, scan.Packet{Data: "$PQTMCFGPPS,OK,1,1*xx\r\n"}, "valset: OK\n$PQTMCFGPPS,OK,1,1*xx\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rh.formatCorrelation(tc.cor, tc.pkt)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func loadRawMsgs(t *testing.T, toml string) []msgfile.RawMsg {
	t.Helper()
	path := t.TempDir() + "/test.toml"
	if err := os.WriteFile(path, []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}
	mf, err := msgfile.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := mf.TaggedMsgs([]string{""})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := msgfile.ToRaw(msgs)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newResponseHandlerWithMsgs(t *testing.T, w io.Writer, toml string) *responseHandler {
	t.Helper()
	rh := newResponseHandler(w, testLogger())
	for _, rm := range loadRawMsgs(t, toml) {
		rh.notifySent(rm)
	}
	return rh
}

func TestUnrecognizedLineBuffer(t *testing.T) {
	var buf bytes.Buffer
	rh := newResponseHandlerWithMsgs(t, &buf, "[[line]]\ntext = \"TEST\"\n")
	rh.bufferLines([]byte("hello\r\n"))
	if buf.String() != "hello\n" {
		t.Errorf("got %q, want %q", buf.String(), "hello\n")
	}
	buf.Reset()
	rh.bufferLines([]byte("part"))
	if buf.String() != "" {
		t.Errorf("expected empty buffer, got %q", buf.String())
	}
	rh.bufferLines([]byte("ial\r\n"))
	if buf.String() != "partial\n" {
		t.Errorf("got %q, want %q", buf.String(), "partial\n")
	}
}

func TestHandleUnrecognizedAnalyze(t *testing.T) {
	unrecognized := scan.Packet{Data: "hello\r\n"} // Format is nil

	// With sent LineMsg: correlator returns MaybeResponse, data displayed.
	var buf bytes.Buffer
	rh := newResponseHandlerWithMsgs(t, &buf, "[[line]]\ntext = \"TEST\"\n")
	rh.handlePacket(unrecognized)
	if buf.String() != "hello\n" {
		t.Errorf("with sent msg: got %q, want %q", buf.String(), "hello\n")
	}

	// Without sent messages: correlator returns NotResponse, data suppressed.
	buf.Reset()
	rh2 := newResponseHandler(&buf, testLogger())
	rh2.handlePacket(unrecognized)
	if buf.String() != "" {
		t.Errorf("without sent msg: got %q, want empty", buf.String())
	}
}
