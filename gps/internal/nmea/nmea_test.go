package nmea

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/lib/opt"
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
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	handled, e := pp.Dispatch(sen, zt, &h)
	if e != nil || !handled {
		t.Fatalf("nmea.Dispatch failed: %v: %s", e, s)
	}
	if !h.utc.IsSet() {
		t.Fatalf("nmea.UTCTime failed: %s", s)
	}
	if utc := h.utc.Get(); utc != expectUTC {
		t.Fatalf("nmea.UTCTime wrong time: %s: got %v, want %v", s, utc, expectUTC)
	}
}

type timeHandler struct {
	gpsprot.DefaultHandler
	utc opt.Val[ptime.UTCTime]
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
	msgs   []gpsprot.Msg
	eoe    bool
}

func (h *mockExtHandler) HandleSentence(_ nmeamsg.SentenceSyntaxFlags, payload string, epoch *NavEpoch) ([]gpsprot.Msg, *NavEpoch, error) {
	if strings.HasPrefix(payload, h.prefix) {
		if h.eoe {
			return h.msgs, nil, nil
		}
		return h.msgs, epoch, nil
	}
	return nil, nil, gpsprot.ErrNotHandled
}

type nativeMsgRecorder struct {
	msgID string
}

func (r *nativeMsgRecorder) NativeMsg(_ gpsprot.Tag, msgID string, _ any, _ time.Time) error {
	r.msgID = msgID
	return nil
}

func TestExtHandlerDispatch(t *testing.T) {
	wantUTC := ptime.UTC(2024, 1, 1, 12, 0, 0, 0)
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.AddExtHandler(&mockExtHandler{
		prefix: "PTEST,",
		msgs:   []gpsprot.Msg{&gpsprot.TimeMsg{Tag: Tag, NativeMsgID: "PTEST", UTCTime: opt.Make(wantUTC)}},
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
	if !th.utc.IsSet() || th.utc.Get() != wantUTC {
		t.Errorf("TimeMsg not dispatched: utc = %v", th.utc)
	}
}

func TestExtHandlerFallthrough(t *testing.T) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.AddExtHandler(&mockExtHandler{
		prefix: "PTEST,",
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
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.AddExtHandler(&mockExtHandler{
		prefix: "PTEST,",
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

// msgRecorder records dispatched messages for testing.
type msgRecorder struct {
	gpsprot.DefaultHandler
	times  []*gpsprot.TimeMsg
	pos    []*gpsprot.PosGeoMsg
	vel    []*gpsprot.VelGeoMsg
	epochs []*gpsprot.NavEpochMsg
}

func (r *msgRecorder) Time(msg *gpsprot.TimeMsg, _ time.Time)     { r.times = append(r.times, msg) }
func (r *msgRecorder) PosGeo(msg *gpsprot.PosGeoMsg, _ time.Time) { r.pos = append(r.pos, msg) }
func (r *msgRecorder) VelGeo(msg *gpsprot.VelGeoMsg, _ time.Time) { r.vel = append(r.vel, msg) }
func (r *msgRecorder) NavEpoch(msg *gpsprot.NavEpochMsg, _ time.Time) {
	cp := *msg
	r.epochs = append(r.epochs, &cp)
}

func approxDeg(a gpsprot.Angle, deg float64) bool {
	return math.Abs(a.Degrees()-deg) < 1e-6
}

func approxSpeed(s gpsprot.Speed, mps float64) bool {
	return math.Abs(s.MetersPerSecond()-mps) < 1e-3
}

func approxLen(l gpsprot.Length, m float64) bool {
	return math.Abs(l.Meters()-m) < 1e-3
}

func TestParseLatLon(t *testing.T) {
	tests := []struct {
		name    string
		lat, ns string
		lon, ew string
		wantLat float64
		wantLon float64
		wantOK  bool
		wantErr bool
	}{
		{"north east", "4717.11437", "N", "00833.91522", "E", 47.0 + 17.11437/60.0, 8.0 + 33.91522/60.0, true, false},
		{"south west", "3855.4487", "S", "09446.0071", "W", -(38.0 + 55.4487/60.0), -(94.0 + 46.0071/60.0), true, false},
		{"equator prime meridian", "0000.0000", "N", "00000.0000", "E", 0, 0, true, false},
		{"empty lat", "", "N", "00833.91522", "E", 0, 0, false, false},
		{"empty lon", "4717.11437", "N", "", "E", 0, 0, false, false},
		{"both empty", "", "N", "", "E", 0, 0, false, false},
		{"bad ns", "4717.11437", "X", "00833.91522", "E", 0, 0, false, true},
		{"bad ew", "4717.11437", "N", "00833.91522", "Q", 0, 0, false, true},
		{"empty ns", "4717.11437", "", "00833.91522", "E", 0, 0, false, true},
		{"empty ew", "4717.11437", "N", "00833.91522", "", 0, 0, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ll, ok, err := parseLatLon(tc.lat, tc.ns, tc.lon, tc.ew)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if !approxDeg(ll[0], tc.wantLat) {
				t.Errorf("lat = %v, want %v", ll[0].Degrees(), tc.wantLat)
			}
			if !approxDeg(ll[1], tc.wantLon) {
				t.Errorf("lon = %v, want %v", ll[1].Degrees(), tc.wantLon)
			}
		})
	}
}

func TestRMCPosVel(t *testing.T) {
	// $GPRMC with active status, position and velocity
	s := "$GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V*2D\r\n"
	sen := parseApprovedSentence(s)
	msgs, epoch, err := parseRMC(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch == nil {
		t.Fatal("expected non-nil epoch")
	}
	var rec msgRecorder
	gpsprot.DispatchMsgs(msgs, &rec, time.Time{})
	if len(rec.times) != 1 {
		t.Fatal("expected 1 TimeMsg")
	}
	if rec.times[0].NativeMsgID != "GPRMC" {
		t.Errorf("NativeMsgID = %q, want GPRMC", rec.times[0].NativeMsgID)
	}
	if rec.times[0].Tag != Tag {
		t.Errorf("Tag = %q, want %q", rec.times[0].Tag, Tag)
	}
	if len(rec.pos) != 1 {
		t.Fatal("expected 1 PosGeoMsg")
	}
	pg := rec.pos[0]
	wantLat := 47.0 + 17.11437/60.0
	wantLon := 8.0 + 33.91522/60.0
	if !approxDeg(pg.LatLon[0], wantLat) {
		t.Errorf("lat = %v, want %v", pg.LatLon[0].Degrees(), wantLat)
	}
	if !approxDeg(pg.LatLon[1], wantLon) {
		t.Errorf("lon = %v, want %v", pg.LatLon[1].Degrees(), wantLon)
	}
	if pg.Height.IsSet() {
		t.Error("RMC should not set Height")
	}
	if pg.HeightMSL.IsSet() {
		t.Error("RMC should not set HeightMSL")
	}
	if len(rec.vel) != 1 {
		t.Fatal("expected 1 VelGeoMsg")
	}
	vg := rec.vel[0]
	wantSpd := 0.004 * 1852.0 / 3600.0
	if !approxSpeed(vg.GroundSpeed.Get(), wantSpd) {
		t.Errorf("GroundSpeed = %v m/s, want %v", vg.GroundSpeed.Get().MetersPerSecond(), wantSpd)
	}
	if !vg.Course.IsSet() || !approxDeg(vg.Course.Get(), 77.52) {
		t.Errorf("Course = %v, want 77.52", vg.Course.Get().Degrees())
	}
}

func TestRMCVoid(t *testing.T) {
	// Status "V" = void/warning: only TimeMsg with nil UTCTime, no pos/vel
	s := "$GPRMC,083559.00,V,,,,,,,091202,,,N*73\r\n"
	sen := parseApprovedSentence(s)
	msgs, epoch, err := parseRMC(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch == nil {
		t.Fatal("expected non-nil epoch")
	}
	var rec msgRecorder
	gpsprot.DispatchMsgs(msgs, &rec, time.Time{})
	if len(rec.times) != 1 {
		t.Fatal("expected 1 TimeMsg")
	}
	if rec.times[0].UTCTime.IsSet() {
		t.Error("expected unset UTCTime for void status")
	}
	if len(rec.pos) != 0 {
		t.Error("expected no PosGeo for void status")
	}
	if len(rec.vel) != 0 {
		t.Error("expected no VelGeo for void status")
	}
}

func TestRMCCourseOnly(t *testing.T) {
	// Speed field empty, course present: should still emit VelGeo
	s := makeSentence("GPRMC,083559.00,A,4717.11437,N,00833.91522,E,,77.52,091202,,,A,V")
	sen := parseApprovedSentence(s)
	msgs, _, err := parseRMC(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	var rec msgRecorder
	gpsprot.DispatchMsgs(msgs, &rec, time.Time{})
	if len(rec.vel) != 1 {
		t.Fatal("expected 1 VelGeoMsg for course-only RMC")
	}
	vg := rec.vel[0]
	if vg.GroundSpeed.IsSet() {
		t.Error("GroundSpeed should not be set when speed field is empty")
	}
	if !vg.Course.IsSet() || !approxDeg(vg.Course.Get(), 77.52) {
		t.Errorf("Course = %v, want 77.52", vg.Course.Get().Degrees())
	}
}

func TestGGAPos(t *testing.T) {
	// GGA with position, MSL height and geoid separation
	s := makeSentence("GNGGA,071113.000,3957.7995312,N,11619.0286230,E,4,16,0.99,103.965,M,-8.408,M,1.0,4042")
	sen := parseApprovedSentence(s)
	pos, epoch, err := parseGGA(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch == nil {
		t.Fatal("expected non-nil epoch")
	}
	if pos == nil {
		t.Fatal("expected non-nil PosGeoMsg")
	}
	if pos.Tag != Tag || pos.NativeMsgID != "GNGGA" {
		t.Errorf("Tag=%q NativeMsgID=%q", pos.Tag, pos.NativeMsgID)
	}
	wantLat := 39.0 + 57.7995312/60.0
	wantLon := 116.0 + 19.0286230/60.0
	if !approxDeg(pos.LatLon[0], wantLat) {
		t.Errorf("lat = %v, want %v", pos.LatLon[0].Degrees(), wantLat)
	}
	if !approxDeg(pos.LatLon[1], wantLon) {
		t.Errorf("lon = %v, want %v", pos.LatLon[1].Degrees(), wantLon)
	}
	if !pos.HeightMSL.IsSet() || !approxLen(pos.HeightMSL.Get(), 103.965) {
		t.Errorf("HeightMSL = %v, want 103.965", pos.HeightMSL.Get().Meters())
	}
	// Ellipsoidal = MSL + geoid sep = 103.965 + (-8.408) = 95.557
	if !pos.Height.IsSet() || !approxLen(pos.Height.Get(), 103.965-8.408) {
		t.Errorf("Height = %v, want %v", pos.Height.Get().Meters(), 103.965-8.408)
	}
}

func TestGGANoFix(t *testing.T) {
	// Quality 0 = no fix
	s := makeSentence("GPGGA,071113.000,,,,,0,00,,,M,,M,,")
	sen := parseApprovedSentence(s)
	pos, epoch, err := parseGGA(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch == nil {
		t.Fatal("expected non-nil epoch")
	}
	if pos != nil {
		t.Error("expected nil PosGeoMsg for quality 0")
	}
}

func TestGGANoGeoidSep(t *testing.T) {
	// GGA with MSL but empty geoid separation
	s := makeSentence("GNGGA,025159.000,3149.29993210,N,11706.91264104,E,1,16,1.26,97.250,M,,M,,")
	sen := parseApprovedSentence(s)
	pos, _, err := parseGGA(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pos == nil {
		t.Fatal("expected non-nil PosGeoMsg")
	}
	if !pos.HeightMSL.IsSet() || !approxLen(pos.HeightMSL.Get(), 97.250) {
		t.Errorf("HeightMSL = %v, want 97.250", pos.HeightMSL.Get().Meters())
	}
	if pos.Height.IsSet() {
		t.Error("Height should not be set when geoid separation is empty")
	}
}

func TestVTGVel(t *testing.T) {
	// VTG with course and speed in km/h
	s := makeSentence("GPVTG,77.52,T,,M,0.004,N,0.007,K,A")
	sen := parseApprovedSentence(s)
	vel, epoch, err := parseVTG(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch == nil {
		t.Fatal("expected non-nil epoch")
	}
	if vel == nil {
		t.Fatal("expected non-nil VelGeoMsg")
	}
	if vel.Tag != Tag || vel.NativeMsgID != "GPVTG" {
		t.Errorf("Tag=%q NativeMsgID=%q", vel.Tag, vel.NativeMsgID)
	}
	wantSpd := 0.007 / 3.6
	if !approxSpeed(vel.GroundSpeed.Get(), wantSpd) {
		t.Errorf("GroundSpeed = %v m/s, want %v", vel.GroundSpeed.Get().MetersPerSecond(), wantSpd)
	}
	if !vel.Course.IsSet() || !approxDeg(vel.Course.Get(), 77.52) {
		t.Errorf("Course = %v, want 77.52", vel.Course.Get().Degrees())
	}
}

func TestVTGNoFix(t *testing.T) {
	// Mode "N" = no fix
	s := makeSentence("GPVTG,,T,,M,,N,,K,N")
	sen := parseApprovedSentence(s)
	vel, epoch, err := parseVTG(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if epoch == nil {
		t.Fatal("expected non-nil epoch")
	}
	if vel != nil {
		t.Error("expected nil VelGeoMsg for mode N")
	}
}

func TestVTGEmptyCourse(t *testing.T) {
	s := makeSentence("GPVTG,,T,,M,,N,5.5,K,A")
	sen := parseApprovedSentence(s)
	vel, _, err := parseVTG(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if vel == nil {
		t.Fatal("expected non-nil VelGeoMsg")
	}
	if vel.Course.IsSet() {
		t.Error("Course should not be set when field is empty")
	}
	wantSpd := 5.5 / 3.6
	if !approxSpeed(vel.GroundSpeed.Get(), wantSpd) {
		t.Errorf("GroundSpeed = %v m/s, want %v", vel.GroundSpeed.Get().MetersPerSecond(), wantSpd)
	}
}

func TestVTGCourseOnly(t *testing.T) {
	// Speed field empty, course present: should still emit VelGeo
	s := makeSentence("GPVTG,123.4,T,,M,,N,,K,A")
	sen := parseApprovedSentence(s)
	vel, _, err := parseVTG(sen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if vel == nil {
		t.Fatal("expected non-nil VelGeoMsg for course-only VTG")
	}
	if vel.GroundSpeed.IsSet() {
		t.Error("GroundSpeed should not be set when speed field is empty")
	}
	if !vel.Course.IsSet() || !approxDeg(vel.Course.Get(), 123.4) {
		t.Errorf("Course = %v, want 123.4", vel.Course.Get().Degrees())
	}
}

// processSentence is a helper that sends a raw NMEA sentence through
// the full PacketProcessor pipeline.
func processSentence(pp *PacketProcessor, payload string, tRead time.Time) error {
	_, err := pp.ProcessPacket(makeSentence(payload), tRead)
	return err
}

func TestEpochBoundary(t *testing.T) {
	var rec msgRecorder
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetMsgHandler(&rec)
	t1 := time.Unix(1000, 0)
	t2 := time.Unix(1001, 0)
	t3 := time.Unix(1002, 0)
	// First epoch: TOD 083559.00
	if err := processSentence(pp, "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V", t1); err != nil {
		t.Fatal(err)
	}
	// No epoch emitted yet (first epoch, no boundary crossed)
	if len(rec.epochs) != 0 {
		t.Fatalf("expected 0 epochs before boundary, got %d", len(rec.epochs))
	}
	// Second epoch: TOD 083600.00 triggers flush of first epoch
	if err := processSentence(pp, "GPRMC,083600.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V", t2); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after boundary, got %d", len(rec.epochs))
	}
	if rec.epochs[0].Tag != Tag {
		t.Errorf("epoch Tag = %q, want %q", rec.epochs[0].Tag, Tag)
	}
	// Third epoch triggers flush of second
	if err := processSentence(pp, "GPRMC,083601.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V", t3); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 2 {
		t.Fatalf("expected 2 epochs, got %d", len(rec.epochs))
	}

}

func TestEpochGGAVTGSameEpoch(t *testing.T) {
	// GGA and VTG within the same epoch should not trigger a flush.
	var rec msgRecorder
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetMsgHandler(&rec)
	t1 := time.Unix(1000, 0)
	if err := processSentence(pp, "GPRMC,120000.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V", t1); err != nil {
		t.Fatal(err)
	}
	if err := processSentence(pp, "GNGGA,120000.00,4717.11437,N,00833.91522,E,1,08,0.9,545.4,M,47.0,M,,", t1); err != nil {
		t.Fatal(err)
	}
	// VTG has no TOD but should stay in same epoch
	if err := processSentence(pp, "GPVTG,77.52,T,,M,0.004,N,0.007,K,A", t1); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 0 {
		t.Fatalf("expected 0 epochs within same epoch, got %d", len(rec.epochs))
	}
	// Verify messages were dispatched
	if len(rec.times) != 1 {
		t.Errorf("expected 1 TimeMsg, got %d", len(rec.times))
	}
	if len(rec.pos) != 2 { // RMC + GGA
		t.Errorf("expected 2 PosGeoMsg, got %d", len(rec.pos))
	}
	if len(rec.vel) != 2 { // RMC + VTG
		t.Errorf("expected 2 VelGeoMsg, got %d", len(rec.vel))
	}
}

// epochMockExtHandler is a mock that participates in the epoch using
// CheckEpoch, for testing ext handler epoch boundaries.
type epochMockExtHandler struct {
	prefix string
	tod    string
	eoe    bool
}

func (h *epochMockExtHandler) HandleSentence(_ nmeamsg.SentenceSyntaxFlags, payload string, epoch *NavEpoch) ([]gpsprot.Msg, *NavEpoch, error) {
	if !strings.HasPrefix(payload, h.prefix) {
		return nil, nil, gpsprot.ErrNotHandled
	}
	if h.eoe {
		return nil, nil, nil
	}
	epoch = CheckEpoch(epoch, h.tod)
	return nil, epoch, nil
}

func TestExtHandlerEpochBoundary(t *testing.T) {
	var rec msgRecorder
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetMsgHandler(&rec)
	pp.AddExtHandler(&epochMockExtHandler{prefix: "PTEST,A", tod: "120000.00"})
	pp.AddExtHandler(&epochMockExtHandler{prefix: "PTEST,B", tod: "120001.00"})
	t1 := time.Unix(1000, 0)
	t2 := time.Unix(1001, 0)
	if err := processSentence(pp, "PTEST,A", t1); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 0 {
		t.Fatalf("expected 0 epochs, got %d", len(rec.epochs))
	}
	// Different TOD triggers boundary
	if err := processSentence(pp, "PTEST,B", t2); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(rec.epochs))
	}
}

func TestExtHandlerEOE(t *testing.T) {
	var rec msgRecorder
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetMsgHandler(&rec)
	pp.AddExtHandler(&epochMockExtHandler{prefix: "PTEST,MSG", tod: "120000.00"})
	pp.AddExtHandler(&epochMockExtHandler{prefix: "PTEST,EOE", eoe: true})
	t1 := time.Unix(1000, 0)
	if err := processSentence(pp, "PTEST,MSG", t1); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 0 {
		t.Fatalf("expected 0 epochs before EOE, got %d", len(rec.epochs))
	}
	// EOE returns (bundle, nil, nil) which triggers flush
	if err := processSentence(pp, "PTEST,EOE", t1); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after EOE, got %d", len(rec.epochs))
	}
}

func TestGGAQuality(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		fixLevel gpsprot.FixLevel
		corr     gpsprot.CorrKind
		auxSrc   gpsprot.AuxSrc
		numSV    uint16
		hasNumSV bool
		hdop     float64
		hasHDOP  bool
		diffAge  gpsprot.Duration
		hasDiff  bool
		refBase  uint16
		hasRef   bool
	}{
		{
			name:     "quality 0 no fix",
			payload:  "GNGGA,071113.000,,,,,0,00,,,M,,M,,",
			fixLevel: gpsprot.FixLevelNone,
		},
		{
			name:     "quality 1 code",
			payload:  "GNGGA,071113.000,3957.7995,N,11619.0286,E,1,08,1.5,100.0,M,-8.0,M,,",
			fixLevel: gpsprot.FixLevelCode,
			numSV:    8,
			hasNumSV: true,
			hdop:     1.5,
			hasHDOP:  true,
		},
		{
			name:     "quality 2 DGPS",
			payload:  "GNGGA,071113.000,3957.7995,N,11619.0286,E,2,10,1.0,100.0,M,-8.0,M,3.0,0001",
			fixLevel: gpsprot.FixLevelCode,
			corr:     gpsprot.CorrUsed,
			numSV:    10,
			hasNumSV: true,
			hdop:     1.0,
			hasHDOP:  true,
			diffAge:  3 * gpsprot.Second,
			hasDiff:  true,
			refBase:  1,
			hasRef:   true,
		},
		{
			name:     "quality 4 RTK fixed",
			payload:  "GNGGA,071113.000,3957.7995,N,11619.0286,E,4,16,0.99,103.965,M,-8.408,M,1.0,4042",
			fixLevel: gpsprot.FixLevelCarrierFixed,
			corr:     gpsprot.CorrOSR | gpsprot.CorrUsed,
			numSV:    16,
			hasNumSV: true,
			hdop:     0.99,
			hasHDOP:  true,
			diffAge:  gpsprot.Second,
			hasDiff:  true,
			refBase:  4042,
			hasRef:   true,
		},
		{
			name:     "quality 5 RTK float",
			payload:  "GNGGA,071113.000,3957.7995,N,11619.0286,E,5,12,1.2,100.0,M,-8.0,M,2.5,1234",
			fixLevel: gpsprot.FixLevelCarrierFloat,
			corr:     gpsprot.CorrOSR | gpsprot.CorrUsed,
			numSV:    12,
			hasNumSV: true,
			hdop:     1.2,
			hasHDOP:  true,
			diffAge:  2500 * gpsprot.Millisecond,
			hasDiff:  true,
			refBase:  1234,
			hasRef:   true,
		},
		{
			name:     "quality 6 DR",
			payload:  "GNGGA,071113.000,3957.7995,N,11619.0286,E,6,00,,,M,,M,,",
			fixLevel: gpsprot.FixLevelNone,
			auxSrc:   gpsprot.AuxSrcDR,
		},
		{
			name:     "quality 7 manual",
			payload:  "GNGGA,071113.000,3957.7995,N,11619.0286,E,7,00,,,M,,M,,",
			fixLevel: gpsprot.FixLevelNotMeasured,
		},
		{
			name:     "ref station over 4095 ignored",
			payload:  "GNGGA,071113.000,3957.7995,N,11619.0286,E,4,16,0.99,103.965,M,-8.408,M,1.0,9901",
			fixLevel: gpsprot.FixLevelCarrierFixed,
			corr:     gpsprot.CorrOSR | gpsprot.CorrUsed,
			numSV:    16,
			hasNumSV: true,
			hdop:     0.99,
			hasHDOP:  true,
			diffAge:  gpsprot.Second,
			hasDiff:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sen := parseApprovedSentence(makeSentence(tc.payload))
			_, epoch, err := parseGGA(sen, nil)
			if err != nil {
				t.Fatal(err)
			}
			if epoch == nil {
				t.Fatal("expected non-nil epoch")
			}
			if epoch.FixLevel != tc.fixLevel {
				t.Errorf("FixLevel = %d, want %d", epoch.FixLevel, tc.fixLevel)
			}
			if epoch.Correction != tc.corr {
				t.Errorf("Correction = %d, want %d", epoch.Correction, tc.corr)
			}
			if epoch.AuxSrc != tc.auxSrc {
				t.Errorf("AuxSrc = %d, want %d", epoch.AuxSrc, tc.auxSrc)
			}
			if tc.hasNumSV {
				if !epoch.NumSVUsed.IsSet() || epoch.NumSVUsed.Get() != tc.numSV {
					t.Errorf("NumSVUsed = %v, want %d", epoch.NumSVUsed, tc.numSV)
				}
			}
			if tc.hasHDOP {
				if !epoch.DOP.Hor.IsSet() || epoch.DOP.Hor.Get() != tc.hdop {
					t.Errorf("DOP.Hor = %v, want %v", epoch.DOP.Hor, tc.hdop)
				}
			}
			if tc.hasDiff {
				if !epoch.DiffAge.IsSet() || epoch.DiffAge.Get() != tc.diffAge {
					t.Errorf("DiffAge = %v, want %v", epoch.DiffAge, tc.diffAge)
				}
			}
			if tc.hasRef {
				if !epoch.RTCMRefBaseID.IsSet() || epoch.RTCMRefBaseID.Get() != tc.refBase {
					t.Errorf("RTCMRefBaseID = %v, want %d", epoch.RTCMRefBaseID, tc.refBase)
				}
			} else if epoch.RTCMRefBaseID.IsSet() {
				t.Errorf("RTCMRefBaseID should not be set, got %d", epoch.RTCMRefBaseID.Get())
			}
		})
	}
}

func TestRMCQuality(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		fixLevel gpsprot.FixLevel
		corr     gpsprot.CorrKind
		auxSrc   gpsprot.AuxSrc
	}{
		{
			name:     "mode N no fix",
			payload:  "GPRMC,083559.00,V,,,,,,,091202,,,N",
			fixLevel: gpsprot.FixLevelNone,
		},
		{
			name:     "mode A autonomous",
			payload:  "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V",
			fixLevel: gpsprot.FixLevelCode,
		},
		{
			name:     "mode D differential",
			payload:  "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,D",
			fixLevel: gpsprot.FixLevelCode,
			corr:     gpsprot.CorrUsed,
		},
		{
			name:     "mode R RTK fixed",
			payload:  "GNRMC,153632.00,A,5550.602949,N,03732.239610,E,000.00000,000.0,310518,,,R,V",
			fixLevel: gpsprot.FixLevelCarrierFixed,
			corr:     gpsprot.CorrOSR | gpsprot.CorrUsed,
		},
		{
			name:     "mode F RTK float",
			payload:  "GNRMC,153632.00,A,5550.602949,N,03732.239610,E,000.00000,000.0,310518,,,F",
			fixLevel: gpsprot.FixLevelCarrierFloat,
			corr:     gpsprot.CorrOSR | gpsprot.CorrUsed,
		},
		{
			name:     "mode P wide area",
			payload:  "GNRMC,153632.00,A,5550.602949,N,03732.239610,E,000.00000,000.0,310518,,,P",
			fixLevel: gpsprot.FixLevelCode,
			corr:     gpsprot.CorrSSR | gpsprot.CorrUsed,
		},
		{
			name:     "mode E dead reckoning",
			payload:  "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,E",
			fixLevel: gpsprot.FixLevelNone,
			auxSrc:   gpsprot.AuxSrcDR,
		},
		{
			name:     "mode M manual",
			payload:  "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,M",
			fixLevel: gpsprot.FixLevelNotMeasured,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sen := parseApprovedSentence(makeSentence(tc.payload))
			_, epoch, err := parseRMC(sen, nil)
			if err != nil {
				t.Fatal(err)
			}
			if epoch == nil {
				t.Fatal("expected non-nil epoch")
			}
			// Extended modes store the letter; finalize applies the override.
			finalizeNavEpoch(epoch)
			if epoch.FixLevel != tc.fixLevel {
				t.Errorf("FixLevel = %d, want %d", epoch.FixLevel, tc.fixLevel)
			}
			if epoch.Correction != tc.corr {
				t.Errorf("Correction = %d, want %d", epoch.Correction, tc.corr)
			}
			if epoch.AuxSrc != tc.auxSrc {
				t.Errorf("AuxSrc = %d, want %d", epoch.AuxSrc, tc.auxSrc)
			}
		})
	}
}

func TestGSAQuality(t *testing.T) {
	// GSA with fix type 3 (3D) and DOPs
	sen := parseApprovedSentence(makeSentence("GNGSA,A,3,01,02,03,04,05,06,07,08,09,10,11,12,1.5,0.9,1.2,1"))
	sb := newSatellitesBuffer()
	_, err := sb.gsaProcess(sen)
	if err != nil {
		t.Fatal(err)
	}
	if sb.gsaFixDim != gpsprot.SolutionDim3D {
		t.Errorf("gsaFixDim = %d, want %d", sb.gsaFixDim, gpsprot.SolutionDim3D)
	}
	if !sb.gsaDOP.Pos.IsSet() || sb.gsaDOP.Pos.Get() != 1.5 {
		t.Errorf("gsaDOP.Pos = %v, want 1.5", sb.gsaDOP.Pos)
	}
	if !sb.gsaDOP.Hor.IsSet() || sb.gsaDOP.Hor.Get() != 0.9 {
		t.Errorf("gsaDOP.Hor = %v, want 0.9", sb.gsaDOP.Hor)
	}
	if !sb.gsaDOP.Vert.IsSet() || sb.gsaDOP.Vert.Get() != 1.2 {
		t.Errorf("gsaDOP.Vert = %v, want 1.2", sb.gsaDOP.Vert)
	}
	// Verify commit copies to epoch
	epoch := &NavEpoch{TimeOfDay: "120000.00"}
	sb.commitGSAQuality(epoch)
	if epoch.SolutionDim != gpsprot.SolutionDim3D {
		t.Errorf("epoch.SolutionDim = %d, want %d", epoch.SolutionDim, gpsprot.SolutionDim3D)
	}
	if !epoch.DOP.Pos.IsSet() || epoch.DOP.Pos.Get() != 1.5 {
		t.Errorf("epoch.DOP.Pos = %v, want 1.5", epoch.DOP.Pos)
	}

	// GSA with fix type 2 (2D)
	sen = parseApprovedSentence(makeSentence("GPGSA,A,2,01,02,03,,,,,,,,,,2.0,1.5,1.3"))
	_, err = sb.gsaProcess(sen)
	if err != nil {
		t.Fatal(err)
	}
	if sb.gsaFixDim != gpsprot.SolutionDim2D {
		t.Errorf("gsaFixDim = %d, want %d", sb.gsaFixDim, gpsprot.SolutionDim2D)
	}

	// GSA with fix type 1 (no fix) should not set SolutionDim
	sb = newSatellitesBuffer()
	sen = parseApprovedSentence(makeSentence("GPGSA,A,1,,,,,,,,,,,,,,,"))
	_, err = sb.gsaProcess(sen)
	if err != nil {
		t.Fatal(err)
	}
	if sb.gsaFixDim != 0 {
		t.Errorf("gsaFixDim = %d, want 0 for no-fix GSA", sb.gsaFixDim)
	}
}

func TestGSAQualityDeferred(t *testing.T) {
	gsaSen := "GNGSA,A,3,01,02,03,04,05,06,07,08,09,10,11,12,1.5,0.9,1.2,1"
	checkGSAQuality := func(t *testing.T, e *gpsprot.NavEpochMsg) {
		t.Helper()
		if e.SolutionDim != gpsprot.SolutionDim3D {
			t.Errorf("SolutionDim = %d, want %d (3D)", e.SolutionDim, gpsprot.SolutionDim3D)
		}
		if !e.DOP.Pos.IsSet() || e.DOP.Pos.Get() != 1.5 {
			t.Errorf("DOP.Pos = %v, want 1.5", e.DOP.Pos)
		}
		if !e.DOP.Hor.IsSet() || e.DOP.Hor.Get() != 0.9 {
			t.Errorf("DOP.Hor = %v, want 0.9", e.DOP.Hor)
		}
		if !e.DOP.Vert.IsSet() || e.DOP.Vert.Get() != 1.2 {
			t.Errorf("DOP.Vert = %v, want 1.2", e.DOP.Vert)
		}
	}
	checkNoGSAQuality := func(t *testing.T, e *gpsprot.NavEpochMsg) {
		t.Helper()
		if e.SolutionDim != 0 {
			t.Errorf("SolutionDim = %d, want 0", e.SolutionDim)
		}
		if e.DOP.Pos.IsSet() {
			t.Errorf("DOP.Pos = %v, want unset", e.DOP.Pos)
		}
	}
	t.Run("GSA_RMC", func(t *testing.T) {
		// GSA arrives before epoch-starting sentence (no epoch active).
		// Quality committed via FlushNavEpoch.
		var rec msgRecorder
		pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
		pp.SetMsgHandler(&rec)
		t1 := time.Unix(1000, 0)
		t2 := time.Unix(1001, 0)
		t3 := time.Unix(1002, 0)
		if err := processSentence(pp, gsaSen, t1); err != nil {
			t.Fatal(err)
		}
		if err := processSentence(pp, "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V", t1); err != nil {
			t.Fatal(err)
		}
		// Flush with next epoch
		if err := processSentence(pp, "GPRMC,083600.00,V,,,,,,,091202,,,N", t2); err != nil {
			t.Fatal(err)
		}
		if len(rec.epochs) != 1 {
			t.Fatalf("expected 1 epoch, got %d", len(rec.epochs))
		}
		checkGSAQuality(t, rec.epochs[0])
		// Second epoch should not carry stale GSA quality
		if err := processSentence(pp, "GPRMC,083601.00,V,,,,,,,091202,,,N", t3); err != nil {
			t.Fatal(err)
		}
		if len(rec.epochs) != 2 {
			t.Fatalf("expected 2 epochs, got %d", len(rec.epochs))
		}
		checkNoGSAQuality(t, rec.epochs[1])
	})
	t.Run("GSA_Idle_RMC", func(t *testing.T) {
		// GSA arrives before epoch, idle gap follows, then RMC starts epoch.
		// Idle flushes with nil epoch; quality must survive for later commit.
		var rec msgRecorder
		pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
		pp.SetMsgHandler(&rec)
		t1 := time.Unix(1000, 0)
		t2 := time.Unix(1001, 0)
		t3 := time.Unix(1002, 0)
		if err := processSentence(pp, gsaSen, t1); err != nil {
			t.Fatal(err)
		}
		pp.Idle(t1)
		if err := processSentence(pp, "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V", t2); err != nil {
			t.Fatal(err)
		}
		// Flush
		if err := processSentence(pp, "GPRMC,083600.00,V,,,,,,,091202,,,N", t3); err != nil {
			t.Fatal(err)
		}
		if len(rec.epochs) != 1 {
			t.Fatalf("expected 1 epoch, got %d", len(rec.epochs))
		}
		checkGSAQuality(t, rec.epochs[0])
	})
	t.Run("RMC_GSA_RMC", func(t *testing.T) {
		// GSA arrives after epoch N starts but belongs to the same burst.
		// Quality attributed to epoch N via FlushNavEpoch.
		var rec msgRecorder
		pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
		pp.SetMsgHandler(&rec)
		t1 := time.Unix(1000, 0)
		t2 := time.Unix(1001, 0)
		if err := processSentence(pp, "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V", t1); err != nil {
			t.Fatal(err)
		}
		if err := processSentence(pp, gsaSen, t1); err != nil {
			t.Fatal(err)
		}
		// Next epoch flushes previous
		if err := processSentence(pp, "GPRMC,083600.00,V,,,,,,,091202,,,N", t2); err != nil {
			t.Fatal(err)
		}
		if len(rec.epochs) != 1 {
			t.Fatalf("expected 1 epoch, got %d", len(rec.epochs))
		}
		checkGSAQuality(t, rec.epochs[0])
	})
}

func TestQualitySynthesis(t *testing.T) {
	// Test that GGA quality overrides RMC basic mode.
	// RMC mode A (code) + GGA quality 4 (RTK fixed) -> epoch gets RTK fixed.
	var rec msgRecorder
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetMsgHandler(&rec)
	t1 := time.Unix(1000, 0)
	t2 := time.Unix(1001, 0)
	// Epoch 1: RMC(A) + GGA(4) + GSA(3D)
	if err := processSentence(pp, "GPRMC,083559.00,A,4717.11437,N,00833.91522,E,0.004,77.52,091202,,,A,V", t1); err != nil {
		t.Fatal(err)
	}
	if err := processSentence(pp, "GNGGA,083559.00,3957.7995,N,11619.0286,E,4,16,0.99,103.965,M,-8.408,M,1.0,4042", t1); err != nil {
		t.Fatal(err)
	}
	if err := processSentence(pp, "GNGSA,A,3,01,02,03,04,05,06,07,08,09,10,11,12,1.5,0.9,1.2,1", t1); err != nil {
		t.Fatal(err)
	}
	// Trigger flush with next epoch
	if err := processSentence(pp, "GPRMC,083600.00,V,,,,,,,091202,,,N", t2); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(rec.epochs))
	}
	e := rec.epochs[0]
	if e.FixLevel != gpsprot.FixLevelCarrierFixed {
		t.Errorf("FixLevel = %d, want %d (CarrierFixed)", e.FixLevel, gpsprot.FixLevelCarrierFixed)
	}
	if e.Correction != gpsprot.CorrOSR|gpsprot.CorrUsed {
		t.Errorf("Correction = %d, want %d", e.Correction, gpsprot.CorrOSR|gpsprot.CorrUsed)
	}
	if e.SolutionDim != gpsprot.SolutionDim3D {
		t.Errorf("SolutionDim = %d, want %d (3D)", e.SolutionDim, gpsprot.SolutionDim3D)
	}
	if !e.NumSVUsed.IsSet() || e.NumSVUsed.Get() != 16 {
		t.Errorf("NumSVUsed = %v, want 16", e.NumSVUsed)
	}
	if !e.DOP.Pos.IsSet() || e.DOP.Pos.Get() != 1.5 {
		t.Errorf("DOP.Pos = %v, want 1.5", e.DOP.Pos)
	}
	// GSA HDOP should overwrite GGA HDOP
	if !e.DOP.Hor.IsSet() || e.DOP.Hor.Get() != 0.9 {
		t.Errorf("DOP.Hor = %v, want 0.9 (from GSA)", e.DOP.Hor)
	}
	if !e.DOP.Vert.IsSet() || e.DOP.Vert.Get() != 1.2 {
		t.Errorf("DOP.Vert = %v, want 1.2", e.DOP.Vert)
	}
	if !e.DiffAge.IsSet() || e.DiffAge.Get() != gpsprot.Second {
		t.Errorf("DiffAge = %v, want 1s", e.DiffAge)
	}
	if !e.RTCMRefBaseID.IsSet() || e.RTCMRefBaseID.Get() != 4042 {
		t.Errorf("RTCMRefBaseID = %v, want 4042", e.RTCMRefBaseID)
	}
}

func TestQualitySynthesisRMCExtendedOverridesGGA(t *testing.T) {
	// RMC extended mode R (RTK fixed) should override GGA quality 2 (DGPS).
	var rec msgRecorder
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	pp.SetMsgHandler(&rec)
	t1 := time.Unix(1000, 0)
	t2 := time.Unix(1001, 0)
	// GGA quality 2 (DGPS) first, then RMC mode R (RTK fixed)
	if err := processSentence(pp, "GNGGA,083559.00,3957.7995,N,11619.0286,E,2,10,1.0,100.0,M,-8.0,M,3.0,0001", t1); err != nil {
		t.Fatal(err)
	}
	if err := processSentence(pp, "GNRMC,083559.00,A,5550.602949,N,03732.239610,E,000.00000,000.0,310518,,,R,V", t1); err != nil {
		t.Fatal(err)
	}
	// Flush
	if err := processSentence(pp, "GNRMC,083600.00,V,,,,,,,310518,,,N", t2); err != nil {
		t.Fatal(err)
	}
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(rec.epochs))
	}
	e := rec.epochs[0]
	// RMC extended mode R should win over GGA quality 2
	if e.FixLevel != gpsprot.FixLevelCarrierFixed {
		t.Errorf("FixLevel = %d, want %d (CarrierFixed from RMC R)", e.FixLevel, gpsprot.FixLevelCarrierFixed)
	}
	if e.Correction != gpsprot.CorrOSR|gpsprot.CorrUsed {
		t.Errorf("Correction = %d, want %d (BaseStation from RMC R)", e.Correction, gpsprot.CorrOSR|gpsprot.CorrUsed)
	}
	// NumSVUsed should still come from GGA
	if !e.NumSVUsed.IsSet() || e.NumSVUsed.Get() != 10 {
		t.Errorf("NumSVUsed = %v, want 10 (from GGA)", e.NumSVUsed)
	}
}
