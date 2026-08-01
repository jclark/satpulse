package gpsio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/bits"
	"slices"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/term"
	"github.com/jclark/satpulse/gps/scan"
)

type trySpeedResult int

const (
	trySilent trySpeedResult = iota
	tryDetected
	tryOther
	tryLower
	tryHigher
)

func (r trySpeedResult) String() string {
	switch r {
	case trySilent:
		return "silent"
	case tryDetected:
		return "detected"
	case tryOther:
		return "other"
	case tryLower:
		return "lower"
	case tryHigher:
		return "higher"
	default:
		return fmt.Sprintf("trySpeedResult(%d)", r)
	}
}

// DetectOutcome classifies a completed detection. It says what the device
// did, not what went wrong: a failure to run the detection at all is an
// error instead.
type DetectOutcome int

const (
	DetectSilent       DetectOutcome = iota // nothing received at any tried speed
	DetectFound                             // a suitable valid packet arrived
	DetectUnrecognized                      // data received, but no speed validated
)

// DetectResult is what a completed detection established. It is meaningful
// only when DetectSpeed returned no error.
type DetectResult struct {
	Outcome DetectOutcome
	Speed   int // the detected speed; set only when Outcome is DetectFound
}

var (
	// ErrNotSerial means the connection is not backed by a terminal.
	ErrNotSerial = errors.New("not a serial device")
	// ErrCurrentSpeedUnknown means the platform reported no current speed
	// for the terminal, so a zero candidate has nothing to resolve to. A
	// speed it does report is accepted even when term.Speed cannot set it.
	ErrCurrentSpeedUnknown = errors.New("current serial speed is unknown")
)

const (
	// These thresholds bracket the separation measured while listening at
	// 38400 to receivers running at the other two common speeds: 9600 produced
	// ratios from 0.14 to 0.20, while 115200 produced 0.40 to 0.49.
	lowerTransitionRatio = 0.30
	upperTransitionRatio = 0.35
	stalePacketMargin    = readTimeout
)

// DefaultSpeedList returns the usual GNSS serial speeds in detection order.
// Zero denotes the speed at which the port was opened.
func DefaultSpeedList() []int {
	return []int{38400, 9600, 115200, 0, 460800, 230400, 57600, 19200, 4800, 921600}
}

type trySpeedStats struct {
	bytes         int
	transitions   int
	pairs         int
	framingErrors int
	readErrors    int
	stalePackets  int
	prevBit       byte
	havePrevBit   bool
}

func (s *trySpeedStats) addData(data string) {
	for _, b := range []byte(data) {
		s.transitions += bits.OnesCount8((b ^ (b >> 1)) & 0x7f)
		s.pairs += 7
		if s.havePrevBit {
			if s.prevBit != b&1 {
				s.transitions++
			}
			s.pairs++
		}
		s.prevBit = (b >> 7) & 1
		s.havePrevBit = true
	}
	s.bytes += len(data)
}

// transitionRatio is the fraction of adjacent received bits that differ.
// Listening too fast samples each transmitted bit repeatedly, producing long
// runs of identical bits and therefore a low ratio.
func (s *trySpeedStats) transitionRatio() float64 {
	if s.pairs == 0 {
		return 0
	}
	return float64(s.transitions) / float64(s.pairs)
}

func (s *trySpeedStats) addPacket(pkt scan.Packet, procs map[gpsprot.Tag]gpsprot.PacketProcessor) trySpeedResult {
	s.addData(pkt.Data)
	if pkt.ReadError != nil && !pkt.IsInterPacketTimeout() {
		s.readErrors++
		s.framingErrors += framingErrorCount(pkt.ReadError)
	}
	if pkt.ChecksumValid && pkt.Format != nil {
		if pp, ok := procs[pkt.Format.Tag()]; ok && !pp.NativeOnly() {
			return tryDetected
		}
	}
	return tryOther
}

func framingErrorCount(err error) int {
	var serialErr interface {
		SerialFraming() bool
	}
	if !errors.As(err, &serialErr) || !serialErr.SerialFraming() {
		return 0
	}
	var termErr *term.Error
	if errors.As(err, &termErr) && termErr.Counts != nil && termErr.Counts.Framing > 0 {
		return int(termErr.Counts.Framing)
	}
	return 1
}

func (s *trySpeedStats) result() trySpeedResult {
	if s.bytes == 0 && s.readErrors == 0 {
		return trySilent
	}
	if s.bytes > 0 {
		ratio := s.transitionRatio()
		if ratio < lowerTransitionRatio {
			return tryLower
		}
		if ratio > upperTransitionRatio {
			return tryHigher
		}
	}
	// A framing error is a weaker signal than a transition ratio outside the
	// ambiguity band. It also shows that a byte-empty window was not silent.
	if s.framingErrors > 0 {
		return tryHigher
	}
	return tryOther
}

func trySpeed(ctx context.Context, packetCh <-chan scan.Packet, procs map[gpsprot.Tag]gpsprot.PacketProcessor, d time.Duration) (trySpeedResult, trySpeedStats, bool, error) {
	start := time.Now()
	cutoff := start.Add(stalePacketMargin)
	timer := time.NewTimer(d)
	defer timer.Stop()
	var stats trySpeedStats
	for {
		select {
		case <-ctx.Done():
			return tryOther, stats, false, ctx.Err()
		case pkt, ok := <-packetCh:
			if !ok {
				if err := ctx.Err(); err != nil {
					return tryOther, stats, false, err
				}
				return stats.result(), stats, true, nil
			}
			if pkt.TRead.Before(cutoff) {
				stats.stalePackets++
				continue
			}
			if stats.addPacket(pkt, procs) == tryDetected {
				return tryDetected, stats, false, nil
			}
		case <-timer.C:
			return stats.result(), stats, false, nil
		}
	}
}

// DetectSpeed looks for a speed at which the device produces suitable valid
// packets. A zero candidate means the speed in effect on entry. On
// DetectFound it leaves the connection at the detected speed. Otherwise the
// speed is unspecified and the caller must close the connection, which
// restores the terminal state captured when the port was opened.
func DetectSpeed(ctx context.Context, lg *slog.Logger, packetCh <-chan scan.Packet, conn *SerialConn, procs map[gpsprot.Tag]gpsprot.PacketProcessor, speeds []int, d time.Duration, stopSilent func(tried []int) bool) (DetectResult, error) {
	if conn == nil {
		panic("nil connection passed to DetectSpeed")
	}
	if lg == nil {
		panic("nil logger passed to DetectSpeed")
	}
	if conn.term() == nil {
		return DetectResult{}, ErrNotSerial
	}
	currentSpeed := conn.Speed()
	candidates, err := resolveSpeedCandidates(speeds, currentSpeed, conn.kind == term.DevUSB)
	if err != nil {
		return DetectResult{}, err
	}
	attempt := func(speed int) (trySpeedResult, bool, error) {
		if speed != currentSpeed {
			if _, err := conn.WriteThenChangeSpeed(nil, speed); err != nil {
				return tryOther, false, fmt.Errorf("changing serial speed to %d: %w", speed, err)
			}
			currentSpeed = speed
			if err := conn.term().Flush(); err != nil {
				return tryOther, false, fmt.Errorf("flushing serial port at %d: %w", speed, err)
			}
		}
		result, stats, streamEnded, err := trySpeed(ctx, packetCh, procs, d)
		lg.Info("tried serial speed",
			"speed", speed,
			"result", result.String(),
			"bytes", stats.bytes,
			"transitionRatio", stats.transitionRatio(),
			"framingErrors", stats.framingErrors,
			"readErrors", stats.readErrors,
			"stalePackets", stats.stalePackets)
		return result, streamEnded, err
	}

	return walkSpeedCandidates(candidates, attempt, stopSilent)
}

func resolveSpeedCandidates(speeds []int, originalSpeed int, devUSB bool) ([]int, error) {
	if originalSpeed <= 0 {
		return nil, ErrCurrentSpeedUnknown
	}
	resolved := make([]int, 0, len(speeds)+2)
	// An entry speed that term.Speed cannot set is reachable only while the
	// port is still on it, so a zero candidate standing for such a speed has
	// to be tried first, ahead even of the DevUSB preference. Anywhere else
	// in the list the speed change would fail and end the walk.
	if !term.IsValidSpeed(originalSpeed) && slices.Contains(speeds, 0) {
		resolved = append(resolved, originalSpeed)
	}
	if devUSB {
		resolved = append(resolved, 115200)
	}
	for _, speed := range speeds {
		if speed == 0 {
			speed = originalSpeed
		}
		resolved = append(resolved, speed)
	}
	return resolved, nil
}

type speedAttempt func(speed int) (trySpeedResult, bool, error)

func walkSpeedCandidates(candidates []int, attempt speedAttempt, stopSilent func([]int) bool) (DetectResult, error) {
	remaining := uniqueSpeedCandidates(candidates)
	tried := make([]int, 0, len(remaining))
	deferred := make(map[int]bool, len(remaining))
	allSilent := true
	for len(remaining) > 0 {
		speed := remaining[0]
		remaining = remaining[1:]
		tried = append(tried, speed)
		result, streamEnded, err := attempt(speed)
		if err != nil {
			return DetectResult{}, err
		}
		if result == tryDetected {
			return DetectResult{Outcome: DetectFound, Speed: speed}, nil
		}
		if result != trySilent {
			allSilent = false
		}
		if streamEnded {
			break
		}
		if result == trySilent && allSilent && stopSilent != nil && stopSilent(slices.Clone(tried)) {
			return DetectResult{Outcome: DetectSilent}, nil
		}
		if shouldSwapNextSpeeds(remaining, deferred, speed, result) {
			deferred[remaining[0]] = true
			remaining[0], remaining[1] = remaining[1], remaining[0]
		}
	}
	if allSilent && len(tried) > 0 {
		return DetectResult{Outcome: DetectSilent}, nil
	}
	return DetectResult{Outcome: DetectUnrecognized}, nil
}

func uniqueSpeedCandidates(speeds []int) []int {
	seen := make(map[int]struct{}, len(speeds))
	unique := make([]int, 0, len(speeds))
	for _, speed := range speeds {
		if _, ok := seen[speed]; ok {
			continue
		}
		seen[speed] = struct{}{}
		unique = append(unique, speed)
	}
	return unique
}

func shouldSwapNextSpeeds(remaining []int, deferred map[int]bool, currentSpeed int, result trySpeedResult) bool {
	if len(remaining) < 2 || deferred[remaining[0]] {
		return false
	}
	return !matchesDirection(remaining[0], currentSpeed, result) &&
		matchesDirection(remaining[1], currentSpeed, result)
}

func matchesDirection(candidate, current int, result trySpeedResult) bool {
	switch result {
	case tryHigher:
		return candidate > current
	case tryLower:
		return candidate < current
	}
	return false
}
