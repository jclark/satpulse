package nmea

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/ptime"
)

func TestSplit(t *testing.T) {
	f := parseApprovedSentence("$GPGLL,5057.970,N,00146.110,E,142451,A*27\r\n")
	df := f.Fields
	if f.TalkerID != "GP" || f.Format != "GLL" || len(df) != 6 ||
		df[0] != "5057.970" || df[1] != "N" || df[2] != "00146.110" ||
		df[3] != "E" || df[4] != "142451" || df[5] != "A" {
		t.Fatalf("NMEASplit failed")
	}
	f = parseApprovedSentence("$GPTXT,1,Hello^21,3*FF\r\n")
	df = f.Fields
	if len(df) != 3 || df[1] != "Hello!" {
		t.Fatalf("NMEASplit failed on caret")
	}
}

func TestUnescape(t *testing.T) {
	if unescape("abc^0D^0Ade^A0f") != "abc\r\nde\u00A0f" {
		t.Fatalf("NMEAUnescape failed")
	}
}

func TestRMC(t *testing.T) {

	testTime(t, "$GPRMC,210230,A,3855.4487,N,09446.0071,W,0.0,076.2,130495,003.8,E*69\r\n", ptime.UTC(1995, 4, 13, 21, 2, 30, 0))
	// This is in the UBX docs, but checksum is 0x57 not 0x2D
	testTime(t, "$GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V*2D\r\n", ptime.UTC(2002, 12, 9, 8, 35, 59, 0))
	testTime(t, "$GPRMC,141632.00,A,5550.6150,N,03732.2523,E,000.00000,243.5,300518,,,A*56\r\n", ptime.UTC(2018, 5, 30, 14, 16, 32, 0))
	testTime(t, "$GNRMC,153632.00,A,5550.602949,N,03732.239610,E,000.00000,000.0,310518,,,R,V*28\r\n", ptime.UTC(2018, 5, 31, 15, 36, 32, 0))
}

func TestZDA(t *testing.T) {
	testTime(t, "$GPZDA,082710.00,16,09,2002,00,00*64\r\n", ptime.UTC(2002, 9, 16, 8, 27, 10, 0))
	testTime(t, "$GPZDA,234500,09,06,1995,-12,45*6C\r\n", ptime.UTC(1995, 6, 9, 23, 45, 0, 0))
}

func testTime(t *testing.T, s string, expectUTC ptime.UTCTime) {
	sen := parseApprovedSentence(s)
	var h timeHandler
	var zt time.Time
	pp := NewPacketProcessor()
	handled, e := pp.Dispatch(sen, zt, &h)
	if e != nil || !handled {
		t.Fatalf("nmea.Dispatch failed: %v: %s", e, s)
	}
	utc := h.utc
	if utc == nil {
		t.Fatalf("nmea.UTCTime failed: %s", s)
	}
	if *utc != expectUTC {
		t.Fatalf("nmea.UTCTime wrong time: %s: got %v, want %v", s, utc, expectUTC)
	}
}

type timeHandler struct {
	gpsprot.DefaultHandler
	utc *ptime.UTCTime
}

func (h *timeHandler) Time(msg *gpsprot.TimeMsg, _ time.Time) {
	h.utc = msg.UTCTime
}

func TestScanTime(t *testing.T) {
	var hour, min, sec uint8
	var nanos int32
	f := func(s string, n int32) {
		if !scanTime("142451"+s, &hour, &min, &sec, &nanos) ||
			hour != 14 || min != 24 || sec != 51 || nanos != n {
			t.Fatalf("NMEAScanTime %s failed", s)
		}
	}
	f("", 0)
	f(".", 0)
	f(".0", 0)
	f(".00", 0)
	f(".0000", 0)
	f(".000000000", 0)
	f(".000000001", 1)
	f(".5", 5e8)

	b := func(s string) {
		if scanTime(s, &hour, &min, &sec, &nanos) {
			t.Fatalf("NMEAScanTime %s succeeded", s)
		}
	}
	b("14245")
	b("14245 ")
	b(" 142451")
	b("142451. ")
	b("14245X")
	b("142451.0000000001")
}

func parseApprovedSentence(data string) *ApprovedSentence {
	sen := NewSentence(data)
	if sen == nil {
		return nil
	}
	return sen.ApprovedSentence()
}

// makeSentence wraps a payload in $ ... *XX\r\n with a correct checksum.
func makeSentence(payload string) string {
	cs := nmeamsg.Checksum([]byte(payload))
	return fmt.Sprintf("$%s*%02X\r\n", payload, cs)
}

// mockExtHandler is a test ExtSentenceHandler that recognizes sentences
// whose payload starts with a given prefix.
type mockExtHandler struct {
	prefix string
	bundle *gpsprot.MsgBundle
	eoe    bool
}

func (h *mockExtHandler) HandleSentence(_ nmeamsg.SentenceSyntaxFlags, payload string, epoch *NavEpoch) (*gpsprot.MsgBundle, *NavEpoch, error) {
	if strings.HasPrefix(payload, h.prefix) {
		if h.eoe {
			return h.bundle, nil, nil
		}
		return h.bundle, epoch, nil
	}
	return nil, nil, nil
}

type nativeMsgRecorder struct {
	msgID string
}

func (r *nativeMsgRecorder) NativeMsg(_ gpsprot.Tag, msgID string, _ interface{}, _ time.Time) error {
	r.msgID = msgID
	return nil
}

func TestExtHandlerDispatch(t *testing.T) {
	wantUTC := ptime.UTC(2024, 1, 1, 12, 0, 0, 0)
	pp := NewPacketProcessor()
	pp.AddExtHandler(&mockExtHandler{
		prefix: "PTEST,",
		bundle: &gpsprot.MsgBundle{
			Time: &gpsprot.TimeMsg{Tag: Tag, NativeMsgID: "PTEST", UTCTime: &wantUTC},
		},
	})
	var th timeHandler
	pp.SetMsgHandler(&th)
	msgID, err := pp.ProcessPacket(makeSentence("PTEST,hello"), time.Time{})
	if err != nil {
		t.Fatalf("ProcessPacket error: %v", err)
	}
	if msgID != "PTEST" {
		t.Errorf("msgID = %q, want PTEST", msgID)
	}
	if th.utc == nil || *th.utc != wantUTC {
		t.Errorf("TimeMsg not dispatched: utc = %v", th.utc)
	}
}

func TestExtHandlerFallthrough(t *testing.T) {
	pp := NewPacketProcessor()
	pp.AddExtHandler(&mockExtHandler{
		prefix: "PTEST,",
		bundle: &gpsprot.MsgBundle{},
	})
	var nr nativeMsgRecorder
	pp.SetNativeMsgHandler(&nr)
	pp.SetMsgHandler(&gpsprot.DefaultHandler{})
	// PFOO is not recognized by the mock handler; should fall through to NativeMsgHandler
	msgID, err := pp.ProcessPacket(makeSentence("PFOO,data"), time.Time{})
	if err != nil {
		t.Fatalf("ProcessPacket error: %v", err)
	}
	if msgID != "PFOO" {
		t.Errorf("msgID = %q, want PFOO", msgID)
	}
	if nr.msgID != "PFOO" {
		t.Errorf("NativeMsgHandler not called: got %q, want PFOO", nr.msgID)
	}
}

func TestExtHandlerBlocksNativeMsg(t *testing.T) {
	pp := NewPacketProcessor()
	pp.AddExtHandler(&mockExtHandler{
		prefix: "PTEST,",
		bundle: &gpsprot.MsgBundle{},
	})
	var nr nativeMsgRecorder
	pp.SetNativeMsgHandler(&nr)
	pp.SetMsgHandler(&gpsprot.DefaultHandler{})
	// PTEST is claimed by the ext handler; NativeMsgHandler should not be called
	_, err := pp.ProcessPacket(makeSentence("PTEST,data"), time.Time{})
	if err != nil {
		t.Fatalf("ProcessPacket error: %v", err)
	}
	if nr.msgID != "" {
		t.Errorf("NativeMsgHandler should not be called when ext handler claims sentence, got %q", nr.msgID)
	}
}
