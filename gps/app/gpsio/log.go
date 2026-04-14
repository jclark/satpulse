package gpsio

import (
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jclark/satpulse/gps/app/logfile"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/scan"
	"golang.org/x/sys/unix"
)

const PacketLogExtension = ".jsonl"

type HexString []byte

type OutPacket struct {
	TWrite time.Time
	Data   string
	Speed  int
}

// NewPacketLog creates a PacketLog and returns the consumer channel.
func NewPacketLog(fmts []gpsprot.PacketFormat) (*PacketLog, <-chan PacketLogEntry) {
	ch := make(chan PacketLogEntry, 2) // 2 because LogInput and LogOutput can be called from separate goroutines
	return &PacketLog{ch: ch, pktFormats: fmts}, ch
}

// LogPackets creates a PacketLog and starts a goroutine that writes entries
// to a JSONL file. Returns nil PacketLog if logPath is empty.
func LogPackets(lg *slog.Logger, wg *sync.WaitGroup, logPath string, append bool, fmts []gpsprot.PacketFormat) (*PacketLog, *logfile.LogFile, error) {
	if logPath == "" {
		return nil, nil, nil
	}
	lf := &logfile.LogFile{}
	err := lf.Open(logPath, append)
	if err != nil {
		return nil, nil, err
	}
	pl, ch := NewPacketLog(fmts)
	wg.Go(func() {
		doLogPackets(lg, lf, ch)
	})
	return pl, lf, nil
}

func doLogPackets(lg *slog.Logger, lf *logfile.LogFile, ch <-chan PacketLogEntry) {
	// Use SIGHUP as a signal to reopen the log file (e.g. after log rotation)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, unix.SIGHUP)
	for {
		select {
		case entry, ok := <-ch:
			if !ok {
				return
			}
			logEntry(lg, lf, &entry)
		case <-sig:
			lf.Reopen(lg)
		}
	}
}

type PacketLogEntry struct {
	T     TimeMicro   `json:"t"`
	Tag   gpsprot.Tag `json:"tag,omitempty"`
	Msg   string      `json:"msg,omitempty"`
	Bin   HexString   `json:"bin,omitempty"`
	Ascii string      `json:"ascii,omitempty"`
	Speed *int        `json:"speed,omitempty"`
	Out   bool        `json:"out,omitzero"`
}

func (ple *PacketLogEntry) Data() string {
	if len(ple.Bin) != 0 {
		return string(ple.Bin)
	}
	return ple.Ascii
}

// TimeMicro is a time.Time that marshals to JSON with 6 fractional digits
type TimeMicro time.Time

const RFC3339Micro = "2006-01-02T15:04:05.999999Z07:00"

// MarshalJSON implements json.Marshaler for TimeMicro
func (t TimeMicro) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(t).UTC().Format(RFC3339Micro))
}

// UnmarshalJSON implements json.Unmarshaler for TimeMilli
func (t *TimeMicro) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(RFC3339Micro, s)
	if err != nil {
		return err
	}
	*t = TimeMicro(parsed)
	return nil
}

// PacketLog provides methods for logging GPS packets.
// It is safe for concurrent use by multiple goroutines.
type PacketLog struct {
	ch         chan<- PacketLogEntry
	pktFormats []gpsprot.PacketFormat
	closeCount atomic.Int32
}

// SemiClose must be called twice, once when LogInput will no longer be called,
// and once when LogOutput will no longer be called.
// The underlying channel is closed when SemiClose has been called twice.
func (pl *PacketLog) SemiClose() {
	if pl.closeCount.Add(1) == 2 {
		close(pl.ch)
	}
}

// LogInput logs an incoming packet from the GPS.
func (pl *PacketLog) LogInput(pkt scan.Packet) {
	if len(pkt.Data) == 0 {
		return
	}
	pl.ch <- MakePacketLogEntry(pkt)
}

// MakePacketLogEntry builds a PacketLogEntry for an incoming scanned packet.
func MakePacketLogEntry(pkt scan.Packet) PacketLogEntry {
	bytes := []byte(pkt.Data)
	msgID := ""
	if pkt.Format != nil {
		msgID = pkt.Format.MsgID(bytes)
	}
	entry := PacketLogEntry{
		T:   TimeMicro(pkt.TRead.UTC()),
		Tag: pkt.Tag(),
		Msg: msgID,
	}
	if useBinary(pkt.Format, bytes) {
		entry.Bin = HexString(bytes)
	} else {
		entry.Ascii = pkt.Data
	}
	return entry
}

func useBinary(fmt gpsprot.PacketFormat, bytes []byte) bool {
	if fmt == nil {
		return containsBinary(bytes)
	}
	return fmt.IsBinary()
}

// LogOutput logs an outgoing write. If fmt is non-nil, it is used directly
// for the tag and message ID. If fmt is nil, pktFormats is iterated to
// discover the format from the raw bytes.
func (pl *PacketLog) LogOutput(tWrite time.Time, bytes []byte, speed int, fmt gpsprot.PacketFormat) {
	if len(bytes) == 0 {
		if speed == 0 {
			return
		}
		fmt = nil // should be nil anyway, but let's be defensive here
	}
	entry := PacketLogEntry{
		T:   TimeMicro(tWrite.UTC()),
		Out: true,
	}
	if speed != 0 {
		entry.Speed = &speed
	}
	if fmt == nil && containsBinary(bytes) {
		for _, pf := range pl.pktFormats {
			if gpsprot.IsValidPacket(pf, bytes) {
				fmt = pf
				break
			}
		}
	}
	if fmt != nil {
		entry.Tag = fmt.Tag()
		entry.Msg = fmt.MsgID(bytes)
	}
	if useBinary(fmt, bytes) {
		entry.Bin = HexString(bytes)
	} else {
		entry.Ascii = string(bytes)
	}
	pl.ch <- entry
}

// PacketWarnAttrs returns slog key-value pairs for logging a packet problem.
func PacketWarnAttrs(err error, pkt scan.Packet, msgID string) []any {
	var attrs []any
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	fmt := pkt.Format
	if fmt != nil {
		attrs = append(attrs, "tag", fmt.Tag())
	}
	if msgID == "" && fmt != nil {
		msgID = fmt.MsgID([]byte(pkt.Data))
	}
	if msgID != "" {
		attrs = append(attrs, "msg", msgID)
	}
	b := []byte(pkt.Data)
	if useBinary(fmt, b) {
		attrs = append(attrs, "bin", HexString(b))
	} else {
		attrs = append(attrs, "ascii", pkt.Data)
	}
	return attrs
}

func containsBinary(data []byte) bool {
	for _, b := range data {
		if (b < 0x20 && b != '\r' && b != '\n') || b >= 0x7F {
			return true
		}
	}
	return false
}

func logEntry(lg *slog.Logger, lf *logfile.LogFile, entry *PacketLogEntry) {
	bytes, err := json.Marshal(entry)
	if err != nil {
		lg.Warn("failed to convert packet to JSON", "err", err)
		return
	}
	bytes = append(bytes, '\n')
	_, err = lf.File.Write(bytes)
	if err != nil {
		lg.Warn("error writing packet to log file", "err", err)
	}
}

// LogValue implements slog.LogValuer for HexString.
func (hs HexString) LogValue() slog.Value {
	return slog.StringValue(hex.EncodeToString(hs))
}

// MarshalJSON implements json.Marshaler for HexString.
func (hs HexString) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(hs))
}

// UnmarshalJSON implements json.Unmarshaler for HexString
// Expects a string of hex digits, two characters per byte
func (hs *HexString) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	bytes, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	*hs = HexString(bytes)
	return nil
}
