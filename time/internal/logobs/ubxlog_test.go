package logobs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

type nativeMsgCall struct {
	tag   gpsprot.Tag
	msgID string
	msg   any
}

type nativeMsgResult struct {
	handled []bool
	entries []map[string]any
}

func TestUBXLogObserverNativeMsg(t *testing.T) {
	tests := []struct {
		name   string
		calls  []nativeMsgCall
		expect nativeMsgResult
	}{
		{
			name: "NAV-TIMETRUSTED logs second values",
			calls: []nativeMsgCall{{
				tag:   gpsreg.TagUBX,
				msgID: "NAV-TIMETRUSTED",
				msg: navTimeTrustedTestMsgWith(func(m *ubxbin.NavTimeTrusted) {
					m.IniTAcc = 1500
					m.PropTAcc = 2500
					m.DeltaS = -2
					m.DeltaMs = -250
				}),
			}},
			expect: nativeMsgResult{
				handled: []bool{true},
				entries: []map[string]any{{
					"level":              "INFO",
					"msg":                "change in receiver's trusted time state",
					"refSys":             float64(1),
					"trustedTimeValid":   true,
					"deltaTimeValid":     true,
					"initialAccuracy":    1.5,
					"propagatedAccuracy": 2.5,
					"deltaTime":          -2.25,
				}},
			},
		},
		{
			name: "SEC-OSNMA authenticating",
			calls: []nativeMsgCall{{
				tag:   gpsreg.TagUBX,
				msgID: "SEC-OSNMA",
				msg:   secOsnmaAuthenticatingTestMsg(8, 4, 2),
			}},
			expect: nativeMsgResult{
				handled: []bool{true},
				entries: []map[string]any{{
					"level":               "INFO",
					"msg":                 "change in receiver's OSNMA state",
					"state":               "authenticating",
					"nmaStatus":           "operational",
					"osnmaSVs":            float64(8),
					"authenticatedSVs":    float64(4),
					"timeSyncEnabled":     true,
					"macAdkdType":         "ADKD12",
					"timingAuth":          "ok",
					"authenticatedTiming": float64(2),
				}},
			},
		},
		{
			name: "SEC-OSNMA DSM alert precedes missing trust material",
			calls: []nativeMsgCall{{
				tag:   gpsreg.TagUBX,
				msgID: "SEC-OSNMA",
				msg: secOsnmaAuthenticatingTestMsgWith(8, 4, 2, func(m *ubxbin.SecOsnma) {
					m.DsmAuthentication = ubxbin.SecOsnmaDsmAuthStatusAlert
					m.GeneralAndTiming &^= ubxbin.SecOsnmaGeneralPubKeyVal | ubxbin.SecOsnmaGeneralMerkleRootVal
				}),
			}},
			expect: nativeMsgResult{
				handled: []bool{true},
				entries: []map[string]any{{
					"level":   "INFO",
					"msg":     "change in receiver's OSNMA state",
					"state":   "dsmFailed",
					"dsmAuth": "alert",
					"cpks":    "nominal",
				}},
			},
		},
		{
			name: "SEC-OSNMA time sync failure logs diff",
			calls: []nativeMsgCall{{
				tag:   gpsreg.TagUBX,
				msgID: "SEC-OSNMA",
				msg: secOsnmaAuthenticatingTestMsgWith(8, 4, 2, func(m *ubxbin.SecOsnma) {
					m.TimSyncReq = ubxbin.SecOsnmaTimSyncEnabled | ubxbin.SecOsnmaTimSyncStatusFailed
					m.TimSyncReqDiff = -1500
				}),
			}},
			expect: nativeMsgResult{
				handled: []bool{true},
				entries: []map[string]any{{
					"level":          "INFO",
					"msg":            "change in receiver's OSNMA state",
					"state":          "timeSyncFailed",
					"timeSyncStatus": "failed",
					"timeSyncDiff":   -1.5,
				}},
			},
		},
		{
			name: "non-UBX message is ignored",
			calls: []nativeMsgCall{{
				tag:   gpsreg.TagNMEA,
				msgID: "NAV-TIMETRUSTED",
				msg:   navTimeTrustedTestMsg(),
			}},
			expect: nativeMsgResult{handled: []bool{false}},
		},
		{
			name: "unrelated UBX message is ignored",
			calls: []nativeMsgCall{{
				tag:   gpsreg.TagUBX,
				msgID: "NAV-TIMEGPS",
				msg:   &ubxbin.NavTimeGPS{},
			}},
			expect: nativeMsgResult{handled: []bool{false}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runNativeMsgCalls(t, tc.calls)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func TestUBXLogObserverNativeMsgSequences(t *testing.T) {
	tests := []struct {
		name   string
		calls  []nativeMsgCall
		expect nativeMsgResult
	}{
		{
			name: "NAV-TIMETRUSTED suppresses until accuracy step or decrease",
			calls: []nativeMsgCall{
				navTimeTrustedTestCall(navTimeTrustedTestMsgWith(func(m *ubxbin.NavTimeTrusted) {
					m.PropTAcc = 1000
				})),
				navTimeTrustedTestCall(navTimeTrustedTestMsgWith(func(m *ubxbin.NavTimeTrusted) {
					m.PropTAcc = 1200
				})),
				navTimeTrustedTestCall(navTimeTrustedTestMsgWith(func(m *ubxbin.NavTimeTrusted) {
					m.PropTAcc = 1500
				})),
				navTimeTrustedTestCall(navTimeTrustedTestMsgWith(func(m *ubxbin.NavTimeTrusted) {
					m.PropTAcc = 1400
				})),
			},
			expect: nativeMsgResult{
				handled: []bool{true, true, true, true},
				entries: []map[string]any{
					{
						"level":              "INFO",
						"msg":                "change in receiver's trusted time state",
						"refSys":             float64(1),
						"trustedTimeValid":   true,
						"deltaTimeValid":     true,
						"initialAccuracy":    float64(0),
						"propagatedAccuracy": float64(1),
						"deltaTime":          float64(0),
					},
					{
						"level":              "INFO",
						"msg":                "change in receiver's trusted time state",
						"refSys":             float64(1),
						"trustedTimeValid":   true,
						"deltaTimeValid":     true,
						"initialAccuracy":    float64(0),
						"propagatedAccuracy": 1.5,
						"deltaTime":          float64(0),
					},
					{
						"level":              "INFO",
						"msg":                "change in receiver's trusted time state",
						"refSys":             float64(1),
						"trustedTimeValid":   true,
						"deltaTimeValid":     true,
						"initialAccuracy":    float64(0),
						"propagatedAccuracy": 1.4,
						"deltaTime":          float64(0),
					},
				},
			},
		},
		{
			name: "NAV-TIMETRUSTED logs state changes",
			calls: []nativeMsgCall{
				navTimeTrustedTestCall(navTimeTrustedTestMsgWith(func(m *ubxbin.NavTimeTrusted) {
					m.PropTAcc = 1000
				})),
				navTimeTrustedTestCall(navTimeTrustedTestMsgWith(func(m *ubxbin.NavTimeTrusted) {
					m.Valid = ubxbin.NavTimeTrustedValidTrustedTime
					m.PropTAcc = 1000
				})),
				navTimeTrustedTestCall(navTimeTrustedTestMsgWith(func(m *ubxbin.NavTimeTrusted) {
					m.RefSys = ubxbin.NavTimeTrustedRefSysGal
					m.Valid = ubxbin.NavTimeTrustedValidTrustedTime
					m.PropTAcc = 1000
				})),
			},
			expect: nativeMsgResult{
				handled: []bool{true, true, true},
				entries: []map[string]any{
					{
						"level":              "INFO",
						"msg":                "change in receiver's trusted time state",
						"refSys":             float64(1),
						"trustedTimeValid":   true,
						"deltaTimeValid":     true,
						"initialAccuracy":    float64(0),
						"propagatedAccuracy": float64(1),
						"deltaTime":          float64(0),
					},
					{
						"level":              "INFO",
						"msg":                "change in receiver's trusted time state",
						"refSys":             float64(1),
						"trustedTimeValid":   true,
						"deltaTimeValid":     false,
						"initialAccuracy":    float64(0),
						"propagatedAccuracy": float64(1),
						"deltaTime":          float64(0),
					},
					{
						"level":              "INFO",
						"msg":                "change in receiver's trusted time state",
						"refSys":             float64(2),
						"trustedTimeValid":   true,
						"deltaTimeValid":     false,
						"initialAccuracy":    float64(0),
						"propagatedAccuracy": float64(1),
						"deltaTime":          float64(0),
					},
				},
			},
		},
		{
			name: "SEC-OSNMA logs count high-water, drop, and recovery",
			calls: []nativeMsgCall{
				secOsnmaTestCall(secOsnmaAuthenticatingTestMsg(8, 4, 2)),
				secOsnmaTestCall(secOsnmaAuthenticatingTestMsg(8, 4, 2)),
				secOsnmaTestCall(secOsnmaAuthenticatingTestMsg(8, 5, 2)),
				secOsnmaTestCall(secOsnmaAuthenticatingTestMsg(8, 3, 2)),
				secOsnmaTestCall(secOsnmaAuthenticatingTestMsg(8, 2, 2)),
				secOsnmaTestCall(secOsnmaAuthenticatingTestMsg(8, 3, 2)),
			},
			expect: nativeMsgResult{
				handled: []bool{true, true, true, true, true, true},
				entries: []map[string]any{
					{
						"level":               "INFO",
						"msg":                 "change in receiver's OSNMA state",
						"state":               "authenticating",
						"nmaStatus":           "operational",
						"osnmaSVs":            float64(8),
						"authenticatedSVs":    float64(4),
						"timeSyncEnabled":     true,
						"macAdkdType":         "ADKD12",
						"timingAuth":          "ok",
						"authenticatedTiming": float64(2),
					},
					{
						"level":               "INFO",
						"msg":                 "change in receiver's OSNMA state",
						"state":               "authenticating",
						"nmaStatus":           "operational",
						"osnmaSVs":            float64(8),
						"authenticatedSVs":    float64(5),
						"timeSyncEnabled":     true,
						"macAdkdType":         "ADKD12",
						"timingAuth":          "ok",
						"authenticatedTiming": float64(2),
					},
					{
						"level":               "INFO",
						"msg":                 "change in receiver's OSNMA state",
						"state":               "authenticating",
						"nmaStatus":           "operational",
						"osnmaSVs":            float64(8),
						"authenticatedSVs":    float64(2),
						"timeSyncEnabled":     true,
						"macAdkdType":         "ADKD12",
						"timingAuth":          "ok",
						"authenticatedTiming": float64(2),
					},
					{
						"level":               "INFO",
						"msg":                 "change in receiver's OSNMA state",
						"state":               "authenticating",
						"nmaStatus":           "operational",
						"osnmaSVs":            float64(8),
						"authenticatedSVs":    float64(3),
						"timeSyncEnabled":     true,
						"macAdkdType":         "ADKD12",
						"timingAuth":          "ok",
						"authenticatedTiming": float64(2),
					},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runNativeMsgCalls(t, tc.calls)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

func newUBXLogObserverTest() (*UBXLogObserver, *bytes.Buffer) {
	var buf bytes.Buffer
	lg := slog.New(slog.NewJSONHandler(&buf, nil))
	return NewUBXLogObserver(lg), &buf
}

func navTimeTrustedTestMsg() *ubxbin.NavTimeTrusted {
	return &ubxbin.NavTimeTrusted{
		Version: 1,
		RefSys:  ubxbin.NavTimeTrustedRefSysGPS,
		Valid:   ubxbin.NavTimeTrustedValidTrustedTime | ubxbin.NavTimeTrustedValidDeltaTime,
	}
}

func navTimeTrustedTestMsgWith(update func(*ubxbin.NavTimeTrusted)) *ubxbin.NavTimeTrusted {
	m := navTimeTrustedTestMsg()
	update(m)
	return m
}

func navTimeTrustedTestCall(msg *ubxbin.NavTimeTrusted) nativeMsgCall {
	return nativeMsgCall{
		tag:   gpsreg.TagUBX,
		msgID: "NAV-TIMETRUSTED",
		msg:   msg,
	}
}

func secOsnmaAuthenticatingTestMsg(osnmaSVs, authenticatedSVs, authenticatedTiming uint8) *ubxbin.SecOsnma {
	return &ubxbin.SecOsnma{
		SecOsnmaFixed: ubxbin.SecOsnmaFixed{
			Version:           3,
			NmaHeader:         ubxbin.SecOsnmaHeaderAuthStatus | ubxbin.SecOsnmaNmaStatusOperational | ubxbin.SecOsnmaCPKSNominal,
			OsnmaMonitoring:   ubxbin.SecOsnmaMonitoringOsnmaEnabled | ubxbin.SecOsnmaMonitoring(osnmaSVs)<<1,
			TimSyncReq:        ubxbin.SecOsnmaTimSyncEnabled | ubxbin.SecOsnmaTimSyncStatusPassed,
			DsmAuthentication: ubxbin.SecOsnmaDsmAuthStatusKROOTOK,
			TeslaKey:          ubxbin.SecOsnmaTeslaKeyAuthOK,
			GeneralAndTiming: ubxbin.SecOsnmaGeneral(authenticatedSVs) |
				ubxbin.SecOsnmaGeneral(authenticatedTiming)<<6 |
				ubxbin.SecOsnmaTimingAuthOK |
				ubxbin.SecOsnmaMacAdkdTypeSlow |
				ubxbin.SecOsnmaGeneralPubKeyVal |
				ubxbin.SecOsnmaGeneralMerkleRootVal,
		},
	}
}

func secOsnmaAuthenticatingTestMsgWith(osnmaSVs, authenticatedSVs, authenticatedTiming uint8, update func(*ubxbin.SecOsnma)) *ubxbin.SecOsnma {
	m := secOsnmaAuthenticatingTestMsg(osnmaSVs, authenticatedSVs, authenticatedTiming)
	update(m)
	return m
}

func secOsnmaTestCall(msg *ubxbin.SecOsnma) nativeMsgCall {
	return nativeMsgCall{
		tag:   gpsreg.TagUBX,
		msgID: "SEC-OSNMA",
		msg:   msg,
	}
}

func runNativeMsgCalls(t *testing.T, calls []nativeMsgCall) nativeMsgResult {
	t.Helper()
	obs, buf := newUBXLogObserverTest()
	result := nativeMsgResult{
		handled: make([]bool, 0, len(calls)),
	}
	for _, call := range calls {
		result.handled = append(result.handled, obs.NativeMsg(call.tag, call.msgID, call.msg, time.Time{}))
	}
	result.entries = readLogEntries(t, buf)
	return result
}

func readLogEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	text := strings.TrimSpace(buf.String())
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to parse log line %q: %v", line, err)
		}
		delete(entry, "time")
		entries = append(entries, entry)
	}
	return entries
}

func TestUBXLogObserverMonCommsAllocWarnRateLimited(t *testing.T) {
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	obs := NewUBXLogObserver(lg)
	tRead := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	msg := &ubxbin.MonComms{
		MonCommsFixed: ubxbin.MonCommsFixed{
			TxErrors: ubxbin.MonCommsTxErrAlloc | ubxbin.MonCommsTxErrors(ubxbin.PortUSB+1)<<2,
		},
	}

	if !obs.NativeMsg(gpsreg.TagUBX, "MON-COMMS", msg, tRead) {
		t.Fatal("NativeMsg returned false")
	}
	output := buf.String()
	if !strings.Contains(output, "GPS receiver reports its transmit buffer has overflowed") {
		t.Fatalf("missing overflow warning in log output %q", output)
	}

	buf.Reset()
	if !obs.NativeMsg(gpsreg.TagUBX, "MON-COMMS", msg, tRead.Add(monCommsAllocWarnInterval-time.Second)) {
		t.Fatal("NativeMsg returned false")
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected warning during rate limit interval: %q", buf.String())
	}

	if !obs.NativeMsg(gpsreg.TagUBX, "MON-COMMS", msg, tRead.Add(monCommsAllocWarnInterval)) {
		t.Fatal("NativeMsg returned false")
	}
	if !strings.Contains(buf.String(), "GPS receiver reports its transmit buffer has overflowed") {
		t.Fatalf("missing overflow warning after rate limit interval in log output %q", buf.String())
	}
}

func TestUBXLogObserverMonCommsNoAllocNoWarn(t *testing.T) {
	var buf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&buf, nil))
	obs := NewUBXLogObserver(lg)
	msg := &ubxbin.MonComms{
		MonCommsFixed: ubxbin.MonCommsFixed{
			TxErrors: ubxbin.MonCommsTxErrMem,
		},
	}

	if !obs.NativeMsg(gpsreg.TagUBX, "MON-COMMS", msg, time.Now()) {
		t.Fatal("NativeMsg returned false")
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected warning without alloc bit: %q", buf.String())
	}
}
