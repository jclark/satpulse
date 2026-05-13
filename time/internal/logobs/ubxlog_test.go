package logobs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func TestUBXLogObserverNativeMsgNavTimeTrustedLogsSecondValues(t *testing.T) {
	obs, buf := newUBXLogObserverTest()
	msg := navTimeTrustedTestMsg()
	msg.IniTAcc = 1500
	msg.PropTAcc = 2500
	msg.DeltaS = -2
	msg.DeltaMs = -250

	if !obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMETRUSTED", msg, time.Time{}) {
		t.Fatal("NativeMsg returned false for NAV-TIMETRUSTED")
	}
	entries := readLogEntries(t, buf)
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry["msg"] != "change in receiver's trusted time state" {
		t.Fatalf("msg = %v, want NAV-TIMETRUSTED log message", entry["msg"])
	}
	if entry["initialAccuracy"] != 1.5 {
		t.Errorf("initialAccuracy = %v, want 1.5", entry["initialAccuracy"])
	}
	if entry["propagatedAccuracy"] != 2.5 {
		t.Errorf("propagatedAccuracy = %v, want 2.5", entry["propagatedAccuracy"])
	}
	if entry["deltaTime"] != -2.25 {
		t.Errorf("deltaTime = %v, want -2.25", entry["deltaTime"])
	}
}

func TestUBXLogObserverNativeMsgNavTimeTrustedSuppressesUntilStepOrDecrease(t *testing.T) {
	obs, buf := newUBXLogObserverTest()
	msg := navTimeTrustedTestMsg()
	msg.PropTAcc = 1000

	obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMETRUSTED", msg, time.Time{})
	assertLogEntryCount(t, buf, 1)

	msg.PropTAcc = 1200
	obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMETRUSTED", msg, time.Time{})
	assertLogEntryCount(t, buf, 1)

	msg.PropTAcc = 1500
	obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMETRUSTED", msg, time.Time{})
	assertLogEntryCount(t, buf, 2)

	msg.PropTAcc = 1400
	obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMETRUSTED", msg, time.Time{})
	assertLogEntryCount(t, buf, 3)
}

func TestUBXLogObserverNativeMsgNavTimeTrustedLogsStateChanges(t *testing.T) {
	obs, buf := newUBXLogObserverTest()
	msg := navTimeTrustedTestMsg()
	msg.PropTAcc = 1000

	obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMETRUSTED", msg, time.Time{})
	assertLogEntryCount(t, buf, 1)

	msg.Valid = ubxbin.NavTimeTrustedValidTrustedTime
	obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMETRUSTED", msg, time.Time{})
	assertLogEntryCount(t, buf, 2)

	msg.RefSys = ubxbin.NavTimeTrustedRefSysGal
	obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMETRUSTED", msg, time.Time{})
	assertLogEntryCount(t, buf, 3)
}

func TestUBXLogObserverNativeMsgFilters(t *testing.T) {
	obs, _ := newUBXLogObserverTest()
	if obs.NativeMsg(gpsreg.TagNMEA, "NAV-TIMETRUSTED", navTimeTrustedTestMsg(), time.Time{}) {
		t.Fatal("NativeMsg returned true for non-UBX NAV-TIMETRUSTED")
	}
	if obs.NativeMsg(gpsreg.TagUBX, "NAV-TIMEGPS", &ubxbin.NavTimeGPS{}, time.Time{}) {
		t.Fatal("NativeMsg returned true for unrelated UBX message")
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

func assertLogEntryCount(t *testing.T, buf *bytes.Buffer, want int) {
	t.Helper()
	if got := len(readLogEntries(t, buf)); got != want {
		t.Fatalf("got %d log entries, want %d\n%s", got, want, buf.String())
	}
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
		entries = append(entries, entry)
	}
	return entries
}
