package unc

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
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

func TestUTCConversionParamsFromGPSUTC(t *testing.T) {
	asciiPackets := []string{
		"#GPSUTCA,97,GPS,FINE,2379,372336000,0,0,18,20;2379,589824,-9.313225746154785e-10,-1.776356839e-15,2441,7,18,18,0,0*8301ddef\r\n",
		"#GPSUTCA,97,GPS,FINE,2381,533672000,17548,0,18,23;2382,61440,-2.793967723846436e-09,-2.664535259e-15,2441,7,18,18,0,0*e1854eff\r\n",
		"#GPSUTCA,97,GPS,FINE,2382,10386000,17548,0,18,22;2382,147456,9.313225746154785e-10,6.217248938e-15,2441,7,18,18,0,0*9e9512e9\r\n",
	}

	for i, asciiPacket := range asciiPackets {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			msg, err := uncmsg.ParseAsciiMessage([]byte(asciiPacket))
			if err != nil {
				t.Fatalf("Failed to parse ASCII packet: %v", err)
			}

			gpsutc, ok := msg.Body.(*uncmsg.GPSUTC)
			if !ok {
				t.Fatalf("Parsed message is not GPSUTC, got %T", msg.Body)
			}

			_, now := msgHdrTime(&msg.Hdr)
			conversion, err := utcConversionParamsFromGPSUTC(gpsutc, now)
			if err != nil {
				t.Fatalf("utcConversionParamsFromGPSUTC failed: %v", err)
			}
			testUTCConversion(t, conversion, &msg.Hdr)
		})
	}
}

func TestUTCConversionParamsFromGALUTC(t *testing.T) {
	asciiPackets := []string{
		"#GALUTCA,97,GPS,FINE,2379,372920000,0,0,18,21;9.313225746154785e-10,0.000000000000000e+00,18,96,1355,1417,7,18,1.513399183750153e-09,-4.440892098500626e-16,345600,11*3729930d\r\n",
		"#GALUTCA,97,GPS,FINE,2381,534975000,17548,0,18,23;9.313225746154785e-10,0.000000000000000e+00,18,144,1357,1417,7,18,2.211891114711761e-09,5.773159728050814e-15,518400,13*23b812c7\r\n",
	}

	for i, asciiPacket := range asciiPackets {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			msg, err := uncmsg.ParseAsciiMessage([]byte(asciiPacket))
			if err != nil {
				t.Fatalf("Failed to parse ASCII packet: %v", err)
			}

			galutc, ok := msg.Body.(*uncmsg.GALUTC)
			if !ok {
				t.Fatalf("Parsed message is not GALUTC, got %T", msg.Body)
			}

			_, now := msgHdrTime(&msg.Hdr)
			conversion, err := utcConversionParamsFromGALUTC(galutc, now)
			if err != nil {
				t.Fatalf("utcConversionParamsFromGALUTC failed: %v", err)
			}
			testUTCConversion(t, conversion, &msg.Hdr)
		})
	}
}

func TestUTCConversionParamsFromBDSUTC(t *testing.T) {
	asciiPackets := []string{
		"#BDSUTCA,97,GPS,FINE,2379,372996000,0,0,18,20;0,0,1.862645149230957e-09,-9.769962617e-15,1085,6,4,4,0,0*2bd1f58c\r\n",
	}

	for i, asciiPacket := range asciiPackets {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			msg, err := uncmsg.ParseAsciiMessage([]byte(asciiPacket))
			if err != nil {
				t.Fatalf("Failed to parse ASCII packet: %v", err)
			}

			bdsutc, ok := msg.Body.(*uncmsg.BDSUTC)
			if !ok {
				t.Fatalf("Parsed message is not BDSUTC, got %T", msg.Body)
			}

			weekStart := msgHdrWeekStart(&msg.Hdr)
			_, now := msgHdrTime(&msg.Hdr)
			conversion, err := utcConversionParamsFromBDSUTC(bdsutc, weekStart, now)
			if err != nil {
				t.Fatalf("utcConversionParamsFromBDSUTC failed: %v", err)
			}
			testUTCConversion(t, conversion, &msg.Hdr)
		})
	}
}

func TestUTCConversionParamsFromBD3UTC(t *testing.T) {
	asciiPackets := []string{
		"#BD3UTCA,97,GPS,FINE,2379,374230000,0,0,18,21;1023,80,1.600710675120354e-09,-1.021405183e-14,0.000000000000000e+00,61,6,4,4,0,0*4b99b719\r\n",
		"#BD3UTCA,97,GPS,FINE,2381,535516000,17548,0,18,23;1025,32,-6.984919309616089e-10,-1.021405183e-14,0.000000000000000e+00,61,6,4,4,0,0*9810311c",
	}

	for i, asciiPacket := range asciiPackets {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			msg, err := uncmsg.ParseAsciiMessage([]byte(asciiPacket))
			if err != nil {
				t.Fatalf("Failed to parse ASCII packet: %v", err)
			}

			bd3utc, ok := msg.Body.(*uncmsg.BD3UTC)
			if !ok {
				t.Fatalf("Parsed message is not BD3UTC, got %T", msg.Body)
			}

			_, now := msgHdrTime(&msg.Hdr)
			conversion, err := utcConversionParamsFromBD3UTC(bd3utc, now)
			if err != nil {
				t.Fatalf("utcConversionParamsFromBD3UTC failed: %v", err)
			}
			testUTCConversion(t, conversion, &msg.Hdr)
		})
	}
}

// TestUTCConversionParamsNoData tests that all-zero UTC messages from a cold
// UM960 receiver (no almanac data) return nil rather than an error.
// Binary packets captured from um960-decoded.jsonl at 10:36:56 UTC.
func TestUTCConversionParamsNoData(t *testing.T) {
	binPackets := map[string]string{
		"GPSUTC": "aa44b5611300300000a06a09907bba11904000000012090000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000095f1e467",
		"GALUTC": "aa44b5611400400000a06a09907bba11904000000012090000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000b86f41e9",
		"BD3UTC": "aa44b5611600380000a06a09907bba1190400000001209000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000e815ce8e",
	}
	for name, h := range binPackets {
		t.Run(name, func(t *testing.T) {
			bin, err := hex.DecodeString(h)
			if err != nil {
				t.Fatal(err)
			}
			msg, err := uncmsg.ParseBinMsg(bin)
			if err != nil {
				t.Fatal(err)
			}
			_, now := msgHdrTime(&msg.Hdr)
			var p *utcConversionParams
			switch body := msg.Body.(type) {
			case *uncmsg.GPSUTC:
				p, err = utcConversionParamsFromGPSUTC(body, now)
			case *uncmsg.GALUTC:
				p, err = utcConversionParamsFromGALUTC(body, now)
			case *uncmsg.BD3UTC:
				p, err = utcConversionParamsFromBD3UTC(body, now)
			default:
				t.Fatalf("unexpected body type %T", body)
			}
			if p != nil || err != nil {
				t.Errorf("got (%v, %v), want (nil, nil)", p, err)
			}
		})
	}
}

func testUTCConversion(t *testing.T, ucp *utcConversionParams, hdr *uncmsg.MsgHdr) {
	gnss, headerTime := msgHdrTime(hdr)
	if gnss == 0 {
		t.Fatal("Expected valid GNSS time from header")
	}

	correction, valid := ucp.Correction.Correction(headerTime)
	
	// Calculate the floating-point correction directly (matching ptime implementation)
	timeDiff := headerTime.Sub(ucp.Correction.Ref)
	fpCorrection := ucp.Correction.Bias + float64(timeDiff)*ucp.Correction.Drift

	t.Logf("Reference time: %v", ucp.Correction.Ref)
	t.Logf("Header time: %v", headerTime)
	t.Logf("Time difference: %v", headerTime.Sub(ucp.Correction.Ref))
	t.Logf("Time difference ns: %v", float64(timeDiff))
	t.Logf("Bias: %v ns", ucp.Correction.Bias)
	t.Logf("Drift: %v", ucp.Correction.Drift)
	t.Logf("FP correction: %v ns", fpCorrection)
	t.Logf("Duration correction: %v", correction)
	t.Logf("Correction valid: %v", valid)

	if !valid {
		t.Errorf("Correction should be valid")
	}

	if fpCorrection == 0 {
		t.Errorf("Floating-point correction should be non-zero for this test case")
	}

	// Verify the LeapSecond matches LeapSecond2016
	expectedLS := ptime.LeapSecond2016()
	if ucp.LeapSecond != expectedLS {
		t.Errorf("LeapSecond = %v, want %v", ucp.LeapSecond, expectedLS)
	}
}
