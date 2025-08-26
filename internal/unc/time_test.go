package unc

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/uncmsg"
)

func TestRecTime(t *testing.T) {
	// Parse the ASCII packet to get the message
	asciiPacket := "#RECTIMEA,97,GPS,FINE,2379,441279000,0,0,18,15;VALID,-4.997042211e-04,7.786425700e-09,-18.00000000000,2025,8,15,2,34,21000,VALID*a15fd8d6\r\n"
	msg, err := uncmsg.ParseAsciiMessage([]byte(asciiPacket))
	if err != nil {
		t.Fatalf("Failed to parse ASCII packet: %v", err)
	}

	_, ok := msg.Body.(*uncmsg.RecTime)
	if !ok {
		t.Fatalf("Parsed message is not RecTime, got %T", msg.Body)
	}

	// Convert to TimeMsg
	timeMsg, err := timeMsgFromRecTime(&msg.Hdr, msg.Body.(*uncmsg.RecTime), TagAscii)
	if err != nil {
		t.Fatalf("timeRecTime failed: %v", err)
	}

	if timeMsg == nil {
		t.Fatal("timeRecTime returned nil")
	}

	// Check the values
	if timeMsg.Tag != TagAscii {
		t.Errorf("Tag = %v, want %v", timeMsg.Tag, TagAscii)
	}

	if timeMsg.NativeMsgID != "RECTIME" {
		t.Errorf("NativeMsgID = %q, want %q", timeMsg.NativeMsgID, "RECTIME")
	}

	if timeMsg.GNSS != gpsprot.GPS {
		t.Errorf("GNSS = %v, want %v", timeMsg.GNSS, gpsprot.GPS)
	}

	// Check UTC time (2025-08-15 02:34:21.000)
	if timeMsg.UTCTime == nil {
		t.Fatal("UTCTime is nil")
	}

	utc := *timeMsg.UTCTime
	expectedUTC := ptime.UTC(2025, 8, 15, 2, 34, 21, 0)
	if utc != expectedUTC {
		t.Errorf("UTCTime = %v, want %v", utc, expectedUTC)
	}

	// Check TAI time - convert the UTC to TAI using leap second info
	// Use the 2016 leap second (UTC offset = 37)
	ls := ptime.LeapSecond2016()
	expectedTAI := ls.UTCtoTime(expectedUTC)
	if timeMsg.TAITime != expectedTAI {
		t.Errorf("TAITime = %v, want %v", timeMsg.TAITime, expectedTAI)
	}

	// Check UTC offset (GPS-UTC = -18, so TAI-UTC = 37)
	if timeMsg.UTCOffset != 37 {
		t.Errorf("UTCOffset = %d, want %d", timeMsg.UTCOffset, 37)
	}

	// Check accuracy (based on OffsetStd = 7.786425700e-09 seconds)
	// convertAccuracy rounds up to avoid underestimating
	expectedAccuracy := time.Duration(8) // 7.786425700 nanoseconds rounds up to 8
	if timeMsg.Accuracy != expectedAccuracy {
		t.Errorf("Accuracy = %v, want %v", timeMsg.Accuracy, expectedAccuracy)
	}
}
