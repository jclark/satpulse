package gpsio

import (
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/logfile"
	"github.com/jclark/satpulse/internal/scan"
	"golang.org/x/sys/unix"
)

const PacketLogExtension = ".jsonl"

type HexString []byte

type OutPacket struct {
	TWrite time.Time
	Data   string
	Speed  int
}

func LogPackets(lg *slog.Logger, wg *sync.WaitGroup, logPath string) (chan<- scan.Packet, chan<- OutPacket, *logfile.LogFile, error) {
	if logPath == "" {
		return nil, nil, nil, nil
	}
	lf := &logfile.LogFile{}
	err := lf.Open(logPath)
	if err != nil {
		return nil, nil, nil, err
	}
	inCh := make(chan scan.Packet, 1)
	outCh := make(chan OutPacket, 1)
	cmd.WaitGroupGo(wg, func() {
		// XXX gpsreg.PacketFormats should be passed in to LogPackets
		doLogPackets(lg, lf, inCh, outCh, gpsreg.PacketFormats)
	})
	return inCh, outCh, lf, nil
}

func doLogPackets(lg *slog.Logger, lf *logfile.LogFile, inCh <-chan scan.Packet, outCh <-chan OutPacket, pktFormats []gpsprot.PacketFormat) {

	// Use SIGHUP as a signal to reopen the log file (e.g. after log rotation)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, unix.SIGHUP)
	for inCh != nil || outCh != nil {
		select {
		case pkt, ok := <-inCh:
			if !ok {
				inCh = nil
				continue
			}
			logPacket(lg, lf, pkt)
		case pkt, ok := <-outCh:
			if !ok {
				outCh = nil
				continue
			}
			logOutPacket(lg, lf, pkt, pktFormats)
		case <-sig:
			lf.Reopen(lg)
		}
	}
}

type PacketLogEntry struct {
	T     TimeMilli   `json:"t"`
	Tag   gpsprot.Tag `json:"tag,omitempty"`
	Msg   string      `json:"msg,omitempty"`
	Bin   HexString   `json:"bin,omitempty"`
	Ascii string      `json:"ascii,omitempty"`
	Speed *int        `json:"speed,omitempty"`
	Out   bool        `json:"out"` // use omitzero here when we upgrade to go 1.24
}

// TimeMilli is a time.Time that marshals to JSON with 3 fractional digits
type TimeMilli time.Time

const RFC3339Milli = "2006-01-02T15:04:05.000Z07:00"

// MarshalJSON implements json.Marshaler for TimeMillis`
func (t TimeMilli) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(t).UTC().Format(RFC3339Milli))
}

func logPacket(lg *slog.Logger, lf *logfile.LogFile, pkt scan.Packet) {
	if len(pkt.Data) == 0 {
		return
	}
	msgID := ""
	bytes := []byte(pkt.Data)
	if pkt.Format != nil {
		msgID = pkt.Format.MsgID(bytes)
	}
	entry := &PacketLogEntry{
		T:   TimeMilli(pkt.TRead.UTC()),
		Tag: pkt.Tag(),
		Msg: msgID,
	}
	if useBinary(pkt) {
		entry.Bin = HexString(bytes)
	} else {
		entry.Ascii = pkt.Data
	}
	logEntry(lg, lf, entry)
}

func useBinary(pkt scan.Packet) bool {
	if pkt.Format == nil {
		return containsBinary(pkt.Data)
	}
	sync1 := pkt.Data[0]
	return sync1 < 0x20 || sync1 >= 0x7F
}

func logOutPacket(lg *slog.Logger, lf *logfile.LogFile, pkt OutPacket, pktFormats []gpsprot.PacketFormat) {
	if len(pkt.Data) == 0 && pkt.Speed == 0 {
		return
	}
	entry := &PacketLogEntry{
		T:   TimeMilli(pkt.TWrite.UTC()),
		Out: true,
	}
	if pkt.Speed != 0 {
		entry.Speed = &pkt.Speed
	}
	if containsBinary(pkt.Data) {
		bytes := []byte(pkt.Data)
		entry.Bin = HexString(bytes)
		for _, pf := range pktFormats {
			if gpsprot.IsValidPacket(pf, bytes) {
				entry.Tag = pf.Tag()
				entry.Msg = pf.MsgID(bytes)
				break
			}
		}
	} else {
		entry.Ascii = pkt.Data
	}
	logEntry(lg, lf, entry)
}

func containsBinary(data string) bool {
	// range over bytes not runes
	for i := 0; i < len(data); i++ {
		if b := data[i]; (b < 0x20 && b != '\r' && b != '\n') || b >= 0x7F {
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

// MarshalJSON implements json.Marshaler for HexString
// It marshals the HexString as a JSON string of hex digits
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
