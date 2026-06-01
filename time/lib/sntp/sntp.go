// Package sntp implements a minimal SNTP client. A single Query
// call performs one request/response exchange with an NTP server
// and returns a Result describing the server's UTC time.
//
// The client does not depend on the local wall-clock being correct.
// The round-trip is measured against the local monotonic clock; the
// absolute time is taken from the server's transmit timestamp. The
// returned Result is anchored at a monotonic instant in
// TimeOfEstimate, so the caller can extrapolate to "now" without
// trusting the local wall-clock.
package sntp

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

// Result is the outcome of a successful Query.
type Result struct {
	Time           time.Time     // estimated UTC wall-clock time at TimeOfEstimate
	TimeOfEstimate time.Time     // monotonic instant the estimate refers to (response receipt)
	Accuracy       time.Duration // upper bound on |Time - true UTC| (local query uncertainty + server root distance)
	Leap           LeapIndicator // leap-second hint reported by the server
}

// LeapIndicator mirrors the NTP leap-indicator field (RFC 5905
// section 7.3) for the values that can appear in a successful
// response. The alarm value (LI=3) is rejected by Query and
// therefore never appears here.
type LeapIndicator uint8

const (
	LeapNone   LeapIndicator = 0 // no warning
	LeapInsert LeapIndicator = 1 // last minute of the day has 61 seconds
	LeapDelete LeapIndicator = 2 // last minute of the day has 59 seconds
)

// secondsBetween1900and1970 is the offset between the NTP epoch
// (1900-01-01 UTC) and the Unix epoch (1970-01-01 UTC).
const secondsBetween1900and1970 = 2208988800

// maxClockAge is the upper bound on tx-minus-reference for a
// trustworthy server response. It corresponds to the maximum NTP
// poll interval (2^17 seconds, ~36.4 hours) per RFC 5905.
const maxClockAge = (1 << 17) * time.Second

// maxRootDistance is RFC 5905's MAXDISP: a server reporting a
// root distance above this is not suitable for synchronization
// (RFC 5905 appendix A.5.1.1).
const maxRootDistance = 16 * time.Second

// rolloverThreshold splits the NTP 32-bit second counter between
// era 0 (1900..2036) and era 1 (2036..2172). NTP seconds smaller
// than this are interpreted as era 1; larger as era 0. The cutoff
// (early 2024 UTC) keeps present-day timestamps in era 0 and won't
// bite until well after the 2036 rollover.
const rolloverThreshold uint32 = 3915710000

// pkt is the on-the-wire NTPv4 packet (48 bytes, big-endian).
type pkt struct {
	LiVnMode       uint8
	Stratum        uint8
	Poll           int8
	Precision      int8
	RootDelay      uint32
	RootDispersion uint32
	RefID          uint32
	RefSec         uint32
	RefFrac        uint32
	OrigSec        uint32
	OrigFrac       uint32
	RxSec          uint32
	RxFrac         uint32
	TxSec          uint32
	TxFrac         uint32
}

// Query performs a single SNTP exchange with server. If server has
// no port, port 123 is used. The whole exchange (DNS resolution,
// dial, send, and receive) is bounded by timeout.
//
// The returned Result has Time set to the server's transmit
// timestamp adjusted by half the round-trip, TimeOfEstimate set to
// the local monotonic instant of response receipt, and Accuracy
// set to a conservative upper bound on |Time - true UTC| that
// combines the local query uncertainty with the server's reported
// root distance.
func Query(server string, timeout time.Duration) (Result, error) {
	addr := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		addr = net.JoinHostPort(server, "123")
	}
	deadline := time.Now().Add(timeout)
	d := net.Dialer{Deadline: deadline}
	conn, err := d.Dial("udp", addr)
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return Result{}, err
	}

	// Random nonce in the request transmit timestamp; the server
	// echoes it in the originate field. Verifying the echo defeats
	// off-path blind injection of forged responses.
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Result{}, err
	}
	req := pkt{LiVnMode: 0x23} // LI=0, VN=4, Mode=3 (client)
	req.TxSec = binary.BigEndian.Uint32(nonce[:4])
	req.TxFrac = binary.BigEndian.Uint32(nonce[4:])

	t1 := time.Now()
	if err := binary.Write(conn, binary.BigEndian, &req); err != nil {
		return Result{}, err
	}
	var resp pkt
	if err := binary.Read(conn, binary.BigEndian, &resp); err != nil {
		return Result{}, err
	}
	t4 := time.Now()

	if resp.OrigSec != req.TxSec || resp.OrigFrac != req.TxFrac {
		return Result{}, errors.New("sntp: nonce mismatch in response")
	}
	li := resp.LiVnMode >> 6
	if li == 3 {
		return Result{}, errors.New("sntp: server alarm (LI=3)")
	}
	if vn := (resp.LiVnMode >> 3) & 7; vn < 3 {
		return Result{}, fmt.Errorf("sntp: bad version %d", vn)
	}
	if mode := resp.LiVnMode & 7; mode != 4 {
		return Result{}, fmt.Errorf("sntp: response mode %d, want 4", mode)
	}
	if resp.Stratum == 0 {
		return Result{}, fmt.Errorf("sntp: kiss-of-death, refID=0x%08x", resp.RefID)
	}
	if resp.Stratum > 15 {
		return Result{}, fmt.Errorf("sntp: bad stratum %d", resp.Stratum)
	}
	if resp.TxSec == 0 && resp.TxFrac == 0 {
		return Result{}, errors.New("sntp: empty transmit timestamp")
	}

	rx := ntpToTime(resp.RxSec, resp.RxFrac)
	tx := ntpToTime(resp.TxSec, resp.TxFrac)
	ref := ntpToTime(resp.RefSec, resp.RefFrac)
	elapsed := t4.Sub(t1)
	serverDt := tx.Sub(rx)
	if serverDt < 0 || serverDt > elapsed {
		return Result{}, errors.New("sntp: implausible server processing time")
	}
	// Freshness: server's last clock update must be recent. The
	// ceiling is the maximum NTP poll interval (~36 hours).
	if age := tx.Sub(ref); age < 0 || age > maxClockAge {
		return Result{}, fmt.Errorf("sntp: stale server clock, last updated %s before tx", age)
	}
	oneway := (elapsed - serverDt) / 2

	// Server's view of true UTC is bounded by its root distance,
	// conventionally rootDelay/2 + rootDispersion (RFC 5905 sec. 11.2);
	// reject if it exceeds MAXDISP (RFC 5905 appendix A.5.1.1).
	rootDist := ntpShortDur(resp.RootDelay)/2 + ntpShortDur(resp.RootDispersion)
	if rootDist > maxRootDistance {
		return Result{}, fmt.Errorf("sntp: server root distance %s exceeds %s", rootDist, maxRootDistance)
	}
	// Accuracy bound: server root distance plus the local query
	// uncertainty, which is bounded by half the round-trip delay
	// (worst-case path asymmetry).
	accuracy := elapsed/2 + rootDist

	return Result{
		Time:           tx.Add(oneway),
		TimeOfEstimate: t4,
		Accuracy:       accuracy,
		Leap:           LeapIndicator(li),
	}, nil
}

// ntpShortDur converts an NTP "short format" value (16-bit
// seconds, 16-bit fractional seconds; used for RootDelay and
// RootDispersion) to a time.Duration.
func ntpShortDur(v uint32) time.Duration {
	return time.Duration((uint64(v) * uint64(time.Second)) >> 16)
}

// ntpToTime converts a 64-bit NTP timestamp (32-bit seconds since
// 1900, 32-bit fractional seconds) to a time.Time, handling the
// 2036 era wrap.
func ntpToTime(sec, frac uint32) time.Time {
	var era int64
	if sec < rolloverThreshold {
		era = 1
	}
	secs := int64(sec) + era*(1<<32) - secondsBetween1900and1970
	nsec := int64((uint64(frac) * 1_000_000_000) >> 32)
	return time.Unix(secs, nsec).UTC()
}
