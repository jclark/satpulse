package gpsio

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
)

const testTag gpsprot.Tag = "TEST"

type testFormat struct{}

func (testFormat) Tag() gpsprot.Tag { return testTag }
func (testFormat) Next(gpsprot.ScanState, []byte, int, int) gpsprot.ScanState {
	return 0
}
func (testFormat) IsFinal(gpsprot.ScanState) bool        { return false }
func (testFormat) MsgID([]byte) string                   { return "" }
func (testFormat) ExtractChecksum([]byte) []byte         { return nil }
func (testFormat) ComputeChecksum([]byte) []byte         { return nil }
func (testFormat) RescanOnBadChecksum(bool, []byte) bool { return false }
func (testFormat) IsBinary() bool                        { return true }

type testProcessor struct {
	gpsprot.DefaultPacketProcessor
	nativeOnly bool
}

func (*testProcessor) ProcessPacket(string, time.Time) (string, error) { return "", nil }
func (p *testProcessor) NativeOnly() bool                              { return p.nativeOnly }

type testTimeoutError struct{}

func (testTimeoutError) Error() string { return "timeout" }
func (testTimeoutError) Timeout() bool { return true }

type testFramingError struct{}

func (testFramingError) Error() string       { return "framing" }
func (testFramingError) SerialFraming() bool { return true }

func TestTransitionRatio(t *testing.T) {
	for _, tc := range []struct {
		data []byte
		want float64
	}{
		{nil, 0},
		{[]byte{0}, 0},
		{[]byte{0x55}, 1},
		{[]byte{0x80, 0x01}, 2.0 / 15.0},
	} {
		var stats trySpeedStats
		stats.addData(string(tc.data))
		if got := stats.transitionRatio(); got != tc.want {
			t.Errorf("transitionRatio(%x) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

func TestTrySpeedStatsResult(t *testing.T) {
	procs := map[gpsprot.Tag]gpsprot.PacketProcessor{testTag: &testProcessor{}}
	for _, tc := range []struct {
		name    string
		packets []scan.Packet
		want    trySpeedResult
	}{
		{name: "silent", packets: []scan.Packet{{ReadError: testTimeoutError{}}}, want: trySilent},
		{name: "valid", packets: []scan.Packet{{Format: testFormat{}, ChecksumValid: true}}, want: tryDetected},
		{name: "high transitions", packets: []scan.Packet{{Data: string([]byte{0x55, 0x55})}}, want: tryHigher},
		{name: "low transitions", packets: []scan.Packet{{Data: string([]byte{0, 0})}}, want: tryLower},
		{name: "ambiguous with framing", packets: []scan.Packet{{Data: string([]byte{0x80, 0x90}), ReadError: testFramingError{}}}, want: tryHigher},
		{name: "ambiguous without framing", packets: []scan.Packet{{Data: string([]byte{0x80, 0x90})}}, want: tryOther},
		{name: "framing without data", packets: []scan.Packet{{ReadError: testFramingError{}}}, want: tryHigher},
		{name: "strong ratio overrides framing", packets: []scan.Packet{{Data: string(bytes.Repeat([]byte{0}, 100)), ReadError: testFramingError{}}}, want: tryLower},
		{name: "nonframing read error", packets: []scan.Packet{{ReadError: errors.New("parity")}}, want: tryOther},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stats trySpeedStats
			for _, pkt := range tc.packets {
				if got := stats.addPacket(pkt, procs); got == tryDetected {
					if tc.want != tryDetected {
						t.Fatalf("addPacket() = detected, want %v", tc.want)
					}
					return
				}
			}
			if got := stats.result(); got != tc.want {
				t.Errorf("result() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTrySpeedRejectsUnsuitableValidPackets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		procs map[gpsprot.Tag]gpsprot.PacketProcessor
	}{
		{name: "no processor", procs: nil},
		{name: "native only", procs: map[gpsprot.Tag]gpsprot.PacketProcessor{testTag: &testProcessor{nativeOnly: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stats trySpeedStats
			pkt := scan.Packet{Format: testFormat{}, Data: string([]byte{0x55}), ChecksumValid: true}
			if got := stats.addPacket(pkt, tc.procs); got == tryDetected {
				t.Fatal("addPacket() detected an unsuitable packet")
			}
		})
	}
}

func TestTrySpeedRecognizesScannedGNSSPackets(t *testing.T) {
	ubx, err := hex.DecodeString("b56201075c00081a021dea07071f0f0a3b3717000000866a01000301ea20212bfd3bb4502f08809cffffb6070000da020000f4030000fffffffffbffffff0100000006000000000000005000000080a81201590000002c2e4c3300000000000000007e4b")
	if err != nil {
		t.Fatal(err)
	}
	formats := gpsreg.CreatePacketFormats(nil)
	procs := gpsreg.CreatePacketProcessors(nil)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "NMEA", data: []byte("$GPGGA,,,,,,0,00,99.99,,,,,,*48\r\n")},
		{name: "UBX", data: ubx},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scanner := scan.New(bytes.NewReader(tc.data), 16, formats)
			pkt, err := scanner.Scan()
			if err != nil {
				t.Fatal(err)
			}
			if pkt.Format == nil || !pkt.ChecksumValid {
				t.Fatalf("scanned packet format/checksum = %v/%v", pkt.Format, pkt.ChecksumValid)
			}
			var stats trySpeedStats
			if got := stats.addPacket(pkt, procs); got != tryDetected {
				t.Errorf("addPacket() = %v, want detected", got)
			}
		})
	}
}

func TestTrySpeedTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		packetCh := make(chan scan.Packet)
		procs := map[gpsprot.Tag]gpsprot.PacketProcessor{testTag: &testProcessor{}}
		go func() {
			packetCh <- scan.Packet{TRead: time.Now().Add(-time.Second), Format: testFormat{}, ChecksumValid: true}
			time.Sleep(stalePacketMargin)
			packetCh <- scan.Packet{TRead: time.Now(), Format: testFormat{}, ChecksumValid: true}
		}()
		got, _, err := trySpeed(context.Background(), packetCh, procs, time.Second)
		if err != nil || got != tryDetected {
			t.Fatalf("trySpeed() = %v, %v, want detected", got, err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			got, _, err := trySpeed(context.Background(), make(chan scan.Packet), nil, time.Second)
			if err != nil || got != trySilent {
				t.Fatalf("trySpeed() = %v, %v, want silent", got, err)
			}
		})
	})
}

func TestTrySpeedErrors(t *testing.T) {
	t.Run("closed channel", func(t *testing.T) {
		ch := make(chan scan.Packet)
		close(ch)
		_, _, err := trySpeed(context.Background(), ch, nil, time.Second)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("trySpeed() error = %v, want io.ErrUnexpectedEOF", err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := trySpeed(ctx, make(chan scan.Packet), nil, time.Second)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("trySpeed() error = %v, want context.Canceled", err)
		}
	})
}

func TestResolveSpeedCandidates(t *testing.T) {
	// 14400 is absent from every platform's speed table, so term.Speed
	// cannot set it; a port reporting it can only be listened to first.
	for _, tc := range []struct {
		name          string
		speeds        []int
		originalSpeed int
		devUSB        bool
		want          []int
	}{
		{"devUSB", []int{38400, 0, 115200}, 57600, true, []int{115200, 38400, 57600, 115200}},
		{"unsettable goes first", []int{38400, 0, 115200}, 14400, false, []int{14400, 38400, 14400, 115200}},
		{"unsettable outranks devUSB", []int{38400, 0}, 14400, true, []int{14400, 115200, 38400, 14400}},
		{"unsettable but not asked for", []int{38400, 115200}, 14400, false, []int{38400, 115200}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSpeedCandidates(tc.speeds, tc.originalSpeed, tc.devUSB)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("resolveSpeedCandidates() = %v, want %v", got, tc.want)
			}
		})
	}
	if _, err := resolveSpeedCandidates([]int{38400, 0}, 0, false); !errors.Is(err, ErrCurrentSpeedUnknown) {
		t.Errorf("resolveSpeedCandidates() error = %v, want ErrCurrentSpeedUnknown", err)
	}
}

func TestWalkSpeedCandidates(t *testing.T) {
	t.Run("direction hints and duplicate", func(t *testing.T) {
		results := map[int]trySpeedResult{
			38400:  tryHigher,
			115200: tryLower,
			9600:   tryOther,
			57600:  tryDetected,
		}
		var order []int
		got, err := walkSpeedCandidates([]int{38400, 9600, 115200, 57600, 115200}, func(speed int) (trySpeedResult, error) {
			order = append(order, speed)
			return results[speed], nil
		}, nil)
		if err != nil || got != (DetectResult{Outcome: DetectFound, Speed: 57600}) {
			t.Fatalf("walkSpeedCandidates() = %+v, %v", got, err)
		}
		wantOrder := []int{38400, 115200, 9600, 57600}
		if fmt.Sprint(order) != fmt.Sprint(wantOrder) {
			t.Errorf("order = %v, want %v", order, wantOrder)
		}
	})
	t.Run("hint does not search past next two", func(t *testing.T) {
		var order []int
		got, err := walkSpeedCandidates([]int{100, 50, 75, 200}, func(speed int) (trySpeedResult, error) {
			order = append(order, speed)
			if speed == 50 {
				return tryDetected, nil
			}
			return tryHigher, nil
		}, nil)
		if err != nil || got.Speed != 50 || fmt.Sprint(order) != "[100 50]" {
			t.Fatalf("walkSpeedCandidates() = %+v, %v; order = %v", got, err, order)
		}
	})
	t.Run("speed is deferred only once", func(t *testing.T) {
		var order []int
		got, err := walkSpeedCandidates([]int{100, 50, 200, 300}, func(speed int) (trySpeedResult, error) {
			order = append(order, speed)
			if speed == 50 {
				return tryDetected, nil
			}
			return tryHigher, nil
		}, nil)
		if err != nil || got.Speed != 50 || fmt.Sprint(order) != "[100 200 50]" {
			t.Fatalf("walkSpeedCandidates() = %+v, %v; order = %v", got, err, order)
		}
	})
	t.Run("silent stop", func(t *testing.T) {
		var order []int
		got, err := walkSpeedCandidates([]int{1, 2, 3}, func(speed int) (trySpeedResult, error) {
			order = append(order, speed)
			return trySilent, nil
		}, func(tried []int) bool { return len(tried) == 2 })
		if err != nil || got.Outcome != DetectSilent || fmt.Sprint(order) != "[1 2]" {
			t.Fatalf("walkSpeedCandidates() = %+v, %v; order = %v", got, err, order)
		}
	})
	t.Run("data exhausted", func(t *testing.T) {
		got, err := walkSpeedCandidates([]int{1}, func(int) (trySpeedResult, error) { return tryOther, nil }, nil)
		if err != nil || got.Outcome != DetectUnrecognized {
			t.Fatalf("walkSpeedCandidates() = %+v, %v", got, err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		got, err := walkSpeedCandidates(nil, func(int) (trySpeedResult, error) { return tryDetected, nil }, nil)
		if err != nil || got.Outcome != DetectUnrecognized {
			t.Fatalf("walkSpeedCandidates() = %+v, %v", got, err)
		}
	})
}
