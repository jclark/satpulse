package gpscmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/jclark/satpulse/gps/msgfile"
	"github.com/jclark/satpulse/gps/scan"
)

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
		name     string
		ackError string
		want     string
	}{
		{"success", "", "cfg/3: OK\n"},
		{"NAK", msgfile.AckNak, "cfg/3: receiver rejected message: NAK\n"},
		{"error text", "Invalid parameter", "cfg/3: receiver rejected message: Invalid parameter\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := msgfile.PacketAnalysis{
				Kind:       msgfile.AckResponse,
				AckError:   tc.ackError,
				RelatedMsg: &rm,
			}
			got := formatAck(r)
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

func TestFormatAnalysis(t *testing.T) {
	rh := &responseHandler{}
	rm := msgfile.RawMsg{Tag: "valset", Index: 0, Count: 1}
	tests := []struct {
		name string
		r    msgfile.PacketAnalysis
		pkt  scan.Packet
		want string
	}{
		{"not response", msgfile.PacketAnalysis{Kind: msgfile.NotResponse}, scan.Packet{}, ""},
		{"ack success", msgfile.PacketAnalysis{Kind: msgfile.AckResponse, RelatedMsg: &rm}, scan.Packet{}, "valset: OK\n"},
		{"ack NAK", msgfile.PacketAnalysis{Kind: msgfile.AckResponse, AckError: msgfile.AckNak, RelatedMsg: &rm}, scan.Packet{}, "valset: receiver rejected message: NAK\n"},
		{"maybe response", msgfile.PacketAnalysis{Kind: msgfile.MaybeResponse}, scan.Packet{Data: "$PTEST,hello*00\r\n"}, "$PTEST,hello*00\n"},
		{"other response", msgfile.PacketAnalysis{Kind: msgfile.OtherResponse}, scan.Packet{Data: "$CONFIG,HEADING,0.0*xx\r\n"}, "$CONFIG,HEADING,0.0*xx\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rh.formatAnalysis(tc.r, tc.pkt)
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
	rh := newResponseHandler(w)
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
	rh.bufferLines([]byte("ial\n"))
	if buf.String() != "partial\n" {
		t.Errorf("got %q, want %q", buf.String(), "partial\n")
	}
}

func TestHandleUnrecognizedAnalyze(t *testing.T) {
	unrecognized := scan.Packet{Data: "hello\r\n"} // Format is nil

	// With sent LineMsg: lineMatcher returns MaybeResponse, data displayed.
	var buf bytes.Buffer
	rh := newResponseHandlerWithMsgs(t, &buf, "[[line]]\ntext = \"TEST\"\n")
	rh.handlePacket(unrecognized)
	if buf.String() != "hello\n" {
		t.Errorf("with sent msg: got %q, want %q", buf.String(), "hello\n")
	}

	// Without sent messages: Analyze returns NotResponse, data suppressed.
	buf.Reset()
	rh2 := newResponseHandler(&buf)
	rh2.handlePacket(unrecognized)
	if buf.String() != "" {
		t.Errorf("without sent msg: got %q, want empty", buf.String())
	}
}
