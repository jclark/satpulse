package gpscfg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/scan"
)

func TestNativeOnlyDetection(t *testing.T) {
	mh := msgHandler{
		lg:          slog.Default(),
		packetProcs: gpsreg.CreatePacketProcessors(gpsreg.VendorUnknown),
		msgCount:    make(map[gpsprot.Tag]int),
	}
	mh.msgCount[gpsreg.TagUBX] = 5
	mh.msgCount[gpsreg.TagNMEA] = 3
	mh.msgCount[gpsreg.TagRTCM] = 7
	suitable := mh.suitableMessageCount()
	if suitable != 8 {
		t.Errorf("suitableMessageCount() = %d, want 8", suitable)
	}
	nativeOnly := mh.nativeOnlyTags()
	if len(nativeOnly) != 1 || nativeOnly[0] != "RTCM" {
		t.Errorf("nativeOnlyTags() = %v, want [RTCM]", nativeOnly)
	}
}

// fakeOutPort implements gpsio.OutPort for testing. Records all writes.
type fakeOutPort struct {
	writes [][]byte
}

var _ gpsio.OutPort = (*fakeOutPort)(nil)

func (p *fakeOutPort) Write(b []byte) (int, error) {
	cp := make([]byte, len(b))
	copy(cp, b)
	p.writes = append(p.writes, cp)
	return len(b), nil
}

func (p *fakeOutPort) Buffered() (int, error)              { return 0, nil }
func (p *fakeOutPort) TransmitTime(nBytes int) time.Duration { return 0 }

// fakeConfigProtocol implements gpsprot.ConfigProtocol for testing.
type fakeConfigProtocol struct {
	probePacket []byte
	probeOK     bool
}

var _ gpsprot.ConfigProtocol = (*fakeConfigProtocol)(nil)

func (p *fakeConfigProtocol) ProbePacket() []byte { return p.probePacket }
func (p *fakeConfigProtocol) ProbeOK() bool       { return p.probeOK }
func (p *fakeConfigProtocol) NativeMsg(gpsprot.Tag, string, any, time.Time) error {
	return nil
}
func (p *fakeConfigProtocol) Configure(*gpsprot.ConfigTarget) (gpsprot.Configurator, error) {
	panic("Configure called in detection test")
}

// fakeSerialError implements SerialError for testing.
type fakeSerialError struct {
	framingErrs int
}

func (e *fakeSerialError) Error() string    { return fmt.Sprintf("%d framing errors", e.framingErrs) }
func (e *fakeSerialError) FramingErrs() int { return e.framingErrs }

// makeNMEAPacket constructs a valid scan.Packet with a GPZDA sentence.
func makeNMEAPacket() scan.Packet {
	body := "GPZDA,120000.00,06,02,2025,00,00"
	cs := nmeamsg.Checksum([]byte(body))
	data := fmt.Sprintf("$%s*%02X\r\n", body, cs)
	return scan.Packet{
		Format:        nmea.PacketFormat,
		Data:          data,
		ChecksumValid: true,
	}
}

// makeFramingErrorPacket constructs a scan.Packet with a framing error.
func makeFramingErrorPacket() scan.Packet {
	return scan.Packet{
		ReadError: &fakeSerialError{framingErrs: 1},
		Data:      "\xff\xff",
	}
}

// setupMsgHandler creates a msgHandler with real packet processors.
func setupMsgHandler(configProts []gpsprot.ConfigProtocol) (*msgHandler, chan scan.Packet) {
	ch := make(chan scan.Packet, 16)
	mh := &msgHandler{}
	mh.init(slog.Default(), gpsreg.CreatePacketProcessors(gpsreg.VendorUnknown), configProts, ch)
	return mh, ch
}

// detectResult holds the return values from detect().
type detectResult struct {
	configProt gpsprot.ConfigProtocol
	err        error
}

// runDetect starts detect() in a goroutine, returning results through a channel.
func runDetect(ctx context.Context, mh *msgHandler, port gpsio.OutPort, probeEnabled, socket bool) <-chan detectResult {
	ch := make(chan detectResult, 1)
	go func() {
		cp, err := mh.detect(ctx, port, probeEnabled, socket)
		ch <- detectResult{cp, err}
	}()
	return ch
}

func TestListeningOnlySuccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mh, packetCh := setupMsgHandler(nil)
		ctx := context.Background()
		resultCh := runDetect(ctx, mh, nil, false, false)
		synctest.Wait()
		packetCh <- makeNMEAPacket()
		synctest.Wait()
		r := <-resultCh
		if r.err != nil {
			t.Fatalf("detect() error = %v, want nil", r.err)
		}
		if r.configProt != nil {
			t.Fatalf("detect() configProt = %v, want nil", r.configProt)
		}
	})
}

func TestListeningOnlyTimeoutWithFramingErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mh, packetCh := setupMsgHandler(nil)
		ctx := context.Background()
		resultCh := runDetect(ctx, mh, nil, false, false)
		synctest.Wait()
		// Send framing errors every 1.5s to keep extending the deadline.
		// The deadline starts at 2s and extends by 2s on each framing error,
		// but never beyond 10s from start.
		// At 3s (after 2 framing errors), the original 2s deadline would have
		// fired if it hadn't been extended. Verify detect is still running.
		for range 2 {
			time.Sleep(1500 * time.Millisecond)
			packetCh <- makeFramingErrorPacket()
			synctest.Wait()
		}
		// t=3s: detect must still be running (deadline was extended past 2s)
		select {
		case <-resultCh:
			t.Fatal("detect() returned at t=3s, want still running (deadline should have extended)")
		default:
		}
		// Continue sending framing errors until past the 10s max deadline
		for range 6 {
			time.Sleep(1500 * time.Millisecond)
			packetCh <- makeFramingErrorPacket()
			synctest.Wait()
		}
		// t=12s: the max deadline (10s) has passed
		r := <-resultCh
		if r.err == nil {
			t.Fatal("detect() error = nil, want ErrNotDetected")
		}
		if !errors.Is(r.err, ErrNotDetected) {
			t.Fatalf("detect() error = %v, want ErrNotDetected", r.err)
		}
	})
}

func TestProbingTriggeredBySilence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fp := &fakeConfigProtocol{probePacket: []byte("PROBE1")}
		mh, _ := setupMsgHandler([]gpsprot.ConfigProtocol{fp})
		port := &fakeOutPort{}
		ctx := context.Background()
		mh.installNativeMsgHandlers()
		resultCh := runDetect(ctx, mh, port, true, false)
		synctest.Wait()
		// Wait 1s for the silence timer to fire and trigger the first probe
		time.Sleep(1 * time.Second)
		synctest.Wait()
		if len(port.writes) != 1 {
			t.Fatalf("expected 1 probe write, got %d", len(port.writes))
		}
		if string(port.writes[0]) != "PROBE1" {
			t.Fatalf("probe write = %q, want %q", port.writes[0], "PROBE1")
		}
		// Let probes time out: 1.5s retry delay + 3s response timeout
		time.Sleep(probeRetryDelay + probeResponseTimeout)
		synctest.Wait()
		// No suitable packets seen, so detect returns ErrNotDetected
		r := <-resultCh
		if !errors.Is(r.err, ErrNotDetected) {
			t.Fatalf("detect() error = %v, want ErrNotDetected", r.err)
		}
		if r.configProt != nil {
			t.Fatalf("detect() configProt = %v, want nil", r.configProt)
		}
		// Two probes should have been sent
		if len(port.writes) != 2 {
			t.Fatalf("expected 2 total probe writes, got %d", len(port.writes))
		}
	})
}

func TestProbingTriggeredByValidPacket(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fp := &fakeConfigProtocol{probePacket: []byte("PROBE1")}
		mh, packetCh := setupMsgHandler([]gpsprot.ConfigProtocol{fp})
		port := &fakeOutPort{}
		ctx := context.Background()
		mh.installNativeMsgHandlers()
		resultCh := runDetect(ctx, mh, port, true, false)
		synctest.Wait()
		// Send a valid NMEA packet before the 1s silence timer.
		// This triggers a probe immediately.
		packetCh <- makeNMEAPacket()
		synctest.Wait()
		if len(port.writes) != 1 {
			t.Fatalf("expected 1 probe write after valid packet, got %d", len(port.writes))
		}
		// Let probes time out: 1.5s retry delay + 3s response timeout
		time.Sleep(probeRetryDelay + probeResponseTimeout)
		synctest.Wait()
		// Suitable packets were seen, so detect returns (nil, nil).
		// The caller (Configure) would return ErrNoProbeResponse.
		r := <-resultCh
		if r.err != nil {
			t.Fatalf("detect() error = %v, want nil", r.err)
		}
		if r.configProt != nil {
			t.Fatalf("detect() configProt = %v, want nil (probe timed out)", r.configProt)
		}
		// Two probes should have been sent
		if len(port.writes) != 2 {
			t.Fatalf("expected 2 total probe writes, got %d", len(port.writes))
		}
	})
}

func TestProbingSuccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fp := &fakeConfigProtocol{probePacket: []byte("PROBE1")}
		mh, packetCh := setupMsgHandler([]gpsprot.ConfigProtocol{fp})
		port := &fakeOutPort{}
		ctx := context.Background()
		mh.installNativeMsgHandlers()
		resultCh := runDetect(ctx, mh, port, true, false)
		synctest.Wait()
		// Send a valid packet to trigger probe
		packetCh <- makeNMEAPacket()
		synctest.Wait()
		// Set probeOK and send another packet so detect re-checks
		fp.probeOK = true
		packetCh <- makeNMEAPacket()
		synctest.Wait()
		r := <-resultCh
		if r.err != nil {
			t.Fatalf("detect() error = %v, want nil", r.err)
		}
		if r.configProt != fp {
			t.Fatalf("detect() configProt = %v, want fakeConfigProtocol", r.configProt)
		}
	})
}

func TestSocketMode(t *testing.T) {
	t.Run("probing", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fp := &fakeConfigProtocol{probePacket: []byte("PROBE1")}
			mh, packetCh := setupMsgHandler([]gpsprot.ConfigProtocol{fp})
			port := &fakeOutPort{}
			ctx := context.Background()
			mh.installNativeMsgHandlers()
			resultCh := runDetect(ctx, mh, port, true, true)
			synctest.Wait()
			// In socket mode, probe is sent immediately
			if len(port.writes) != 1 {
				t.Fatalf("expected 1 probe write in socket mode, got %d", len(port.writes))
			}
			// Set probeOK and send a packet
			fp.probeOK = true
			packetCh <- makeNMEAPacket()
			synctest.Wait()
			r := <-resultCh
			if r.err != nil {
				t.Fatalf("detect() error = %v, want nil", r.err)
			}
			if r.configProt != fp {
				t.Fatalf("detect() configProt = %v, want fakeConfigProtocol", r.configProt)
			}
		})
	})
	t.Run("no-probe", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			mh, _ := setupMsgHandler(nil)
			ctx := context.Background()
			resultCh := runDetect(ctx, mh, nil, false, true)
			synctest.Wait()
			r := <-resultCh
			if r.err != nil {
				t.Fatalf("detect() error = %v, want nil", r.err)
			}
			if r.configProt != nil {
				t.Fatalf("detect() configProt = %v, want nil", r.configProt)
			}
		})
	})
}

func TestProbeRetryAndTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fp := &fakeConfigProtocol{probePacket: []byte("PROBE1")}
		mh, packetCh := setupMsgHandler([]gpsprot.ConfigProtocol{fp})
		port := &fakeOutPort{}
		ctx := context.Background()
		mh.installNativeMsgHandlers()
		resultCh := runDetect(ctx, mh, port, true, false)
		synctest.Wait()
		// Send a valid packet to trigger first probe immediately
		packetCh <- makeNMEAPacket()
		synctest.Wait()
		if len(port.writes) != 1 {
			t.Fatalf("after first packet: expected 1 probe write, got %d", len(port.writes))
		}
		// After 1.5s (probeRetryDelay), second probe should be sent
		time.Sleep(probeRetryDelay)
		synctest.Wait()
		if len(port.writes) != 2 {
			t.Fatalf("after retry delay: expected 2 probe writes, got %d", len(port.writes))
		}
		// Detect should still be running (waiting for probeResponseTimeout)
		select {
		case <-resultCh:
			t.Fatal("detect() returned after 2nd probe, want still waiting for response timeout")
		default:
		}
		// After 3s more (probeResponseTimeout), detect gives up
		time.Sleep(probeResponseTimeout)
		synctest.Wait()
		r := <-resultCh
		// Suitable packets were seen, so detect returns (nil, nil)
		if r.err != nil {
			t.Fatalf("detect() error = %v, want nil", r.err)
		}
		if r.configProt != nil {
			t.Fatalf("detect() configProt = %v, want nil (probe timed out)", r.configProt)
		}
	})
}

func TestSilenceTimerCancelsOnInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fp := &fakeConfigProtocol{probePacket: []byte("PROBE1")}
		mh, packetCh := setupMsgHandler([]gpsprot.ConfigProtocol{fp})
		port := &fakeOutPort{}
		ctx := context.Background()
		mh.installNativeMsgHandlers()
		resultCh := runDetect(ctx, mh, port, true, false)
		synctest.Wait()
		// Send a framing error at 500ms. This is input, so it should
		// permanently cancel the silence timer.
		time.Sleep(500 * time.Millisecond)
		packetCh <- makeFramingErrorPacket()
		synctest.Wait()
		// No probe should have been sent yet (silence timer cancelled,
		// no valid packet received, deadline hasn't fired)
		if len(port.writes) != 0 {
			t.Fatalf("after framing error at 500ms: expected 0 probe writes, got %d", len(port.writes))
		}
		// Advance past the original 1s silence timer. No probe should fire.
		time.Sleep(600 * time.Millisecond)
		synctest.Wait()
		if len(port.writes) != 0 {
			t.Fatalf("at 1.1s: expected 0 probe writes (silence timer cancelled), got %d", len(port.writes))
		}
		// The deadline (extended by framing error) eventually fires and triggers probe.
		// Deadline was extended to 2s from the framing error at 0.5s = 2.5s.
		time.Sleep(1400 * time.Millisecond)
		synctest.Wait()
		if len(port.writes) != 1 {
			t.Fatalf("after deadline: expected 1 probe write, got %d", len(port.writes))
		}
		// Let probes finish
		time.Sleep(probeRetryDelay + probeResponseTimeout)
		synctest.Wait()
		r := <-resultCh
		if !errors.Is(r.err, ErrNotDetected) {
			t.Fatalf("detect() error = %v, want ErrNotDetected", r.err)
		}
	})
}
