package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/tsync"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/ptime"
	"github.com/jclark/gps2phc/ubx"
	"github.com/pkg/term"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

var serialDev string
var ifName string
var debugEnable bool

type Syncer struct {
	tsCh <-chan phc.TsEvent
	fCh  chan scan.Frame
	corr *tsync.Correlator
}

func main() {
	flag.StringVar(&serialDev, "s", "/dev/ttyUSB0", "device for serial connection to GPS")
	flag.StringVar(&ifName, "e", "eth0", "ethernet interface of PTP hardware clock")
	flag.BoolVar(&debugEnable, "d", false, "log debuggging information")
	flag.Parse()
	level := slog.LevelInfo
	if debugEnable {
		level = slog.LevelDebug
	}
	lg := slog.New(slog.HandlerOptions{Level: level}.NewTextHandler(os.Stdout))
	slog.SetDefault(lg)
	ctx := context.Background()
	ctx = cancelOnSignal(ctx)
	clk, err := openExttsClock()
	var t *term.Term
	if err == nil {
		lg.Debug("serial", "devType", serDevType(serialDev))
		t, err = serOpen(ctx, serialDev)
	}
	var fCh chan scan.Frame
	if err == nil {
		fCh, err = gpsInit(ctx, t)
	}
	var s *Syncer
	if err == nil {
		s, err = newSyncer(ctx, clk, fCh)
	}
	// XXX if fCh is nil and err is non-nil,
	// we should receive from fCh until it is closed
	// At that point, we can cleanup and exit
	if err != nil {
		slog.Error("exiting", err)
	} else {
		doSync(ctx, s)
		slog.Debug("exiting")
		err = t.Restore()
		if err != nil {
			slog.Error("could not restore terminal settings", err)
		}
		t.Close()
		slog.Debug("closed", "serial", serialDev)
		clk.Close()
		slog.Debug("closed", "if", ifName)
	}
}

func cancelOnSignal(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		slog.FromContext(ctx).Debug("cancelling")
		cancel()
	}()
	return ctx
}

func openExttsClock() (*phc.Clock, error) {
	phcIndex, err := phc.IfPhcIndex(ifName)
	if err != nil {
		return nil, err
	}
	if phcIndex < 0 {
		return nil, fmt.Errorf("interface %s cannot be used because it does not have a PTP hardware clock", ifName)
	}
	clk, err := phc.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, err
	}
	if clk.ExttsChanCount() == 0 {
		clk.Close()
		return nil, fmt.Errorf("interface %s does not support external timestamping", ifName)
	}
	return clk, nil
}

func gpsInit(ctx context.Context, t *term.Term) (frameCh chan scan.Frame, err error) {
	frameCh = serReadStart(ctx, scan.New(t, 16))
	// must wait for writeRespCh before returning
	// so the called can close the Term without a data race
	configMsgs := [][]byte{
		ubx.Poll[ubx.MonVer](),
		ubx.Poll[ubx.CfgTmode2](),
		ubx.Poll[ubx.CfgTp5](),
		ubx.Poll[ubx.TimSvin](),
		ubx.SetRate[ubx.NavTimeGPS](1),
		ubx.SetRate[ubx.TimTP](1),
	}
	writeRespCh := serWriteAsync(ctx, t, configMsgs)
	timerCh := time.After(time.Second * 2)
	cancelCh := ctx.Done()
	nmeaMsgs := []string{}
	ubxMsgs := []string{}
	invalidByteCount := 0
	for {
		select {
		case frame, ok := <-frameCh:
			if ok {
				switch frame.Kind {
				case scan.NMEA:
					nmeaMsgs = append(nmeaMsgs, string(frame.Data))
				case scan.UBX:
					ubxMsgs = append(ubxMsgs, string(frame.Data))
				case scan.Invalid:
					invalidByteCount += len(frame.Data)
				}
			} else {
				frameCh = nil
			}
		case <-cancelCh:
			cancelCh = nil
			if err != nil {
				err = ctx.Err()
			}
		case e := <-writeRespCh:
			writeRespCh = nil
			if e != nil && err == nil {
				err = e
			}
		case <-timerCh:
			timerCh = nil
		}
		if writeRespCh == nil {
			if err != nil {
				return
			}
			if timerCh == nil || frameCh == nil {
				break
			}
		}
	}
	lg := slog.FromContext(ctx)
	if len(ubxMsgs) == 0 && len(nmeaMsgs) == 0 {
		if invalidByteCount == 0 {
			err = errors.New("new output detected from GPS")
		} else {
			err = errors.New("could not understand GPS output")
		}
		return
	}
	for _, msg := range ubxMsgs {
		u, err := ubx.ParseMsg(msg)
		if err != nil {
			lg.Error("ubxParseError", err)
		} else if u != nil {
			switch data := u.(type) {
			case *ubx.MonVer:
				major, minor := data.ProtVer()
				protVer := "?"
				if major >= 0 {
					protVer = fmt.Sprintf("%d.%02d", major, minor)
				}
				lg.Info("gpsVersion", "sw", ubx.Latin1ZToString(data.SwVersion[:]), "hw", ubx.Latin1ZToString(data.HwVersion[:]), "protver", protVer)
			default:
				lg.Debug("ubx", "type", u.ID().String(), "payload", u)
			}
		}
	}
	for _, msg := range nmeaMsgs {
		nmeaLog(lg, msg)
	}
	lg.Debug("gpsInitDone")
	return
}

func nmeaLog(lg *slog.Logger, data string) {
	fields := scan.NMEASplit(data)
	if fields.SentenceFmt == "TXT" && len(fields.DataFields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Info("nmeaTxt", "s", fields.DataFields[3])
	}
}

func newSyncer(ctx context.Context, clk *phc.Clock, fCh chan scan.Frame) (r *Syncer, err error) {
	err = nil
	r = nil
	lg := slog.FromContext(ctx)

	servo, err := tsync.NewServo(clk, lg)
	if err != nil {
		return nil, err
	}
	s := Syncer{corr: tsync.NewCorrelator(servo), fCh: fCh}
	lg.Info("usingPHC", "path", clk.Path())
	s.tsCh, err = StartPPS(ctx, clk)
	if err != nil {
		return
	}
	r = &s
	return
}

func doSync(ctx context.Context, s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	fCh := s.fCh
	corr := s.corr
	lg := slog.FromContext(ctx)
	nSkipped := 0
	for tsCh != nil || fCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				if e.Epoch == ptime.InitialEpoch {
					if nSkipped == 0 {
						lg.Info("stalePHCTimestamps", "t", e.T)
					}
					nSkipped++
				} else {
					if nSkipped > 0 {
						lg.Info("skippedStalePHCTimestamps", "n", nSkipped)
						nSkipped = 0
					}
					corr.PulseEdge(e.ClockTime, e.TRead)
				}
			} else {
				tsCh = nil
			}
		case f, ok := <-fCh:
			if ok {
				syncFrame(ctx, corr, f)
			} else {
				fCh = nil
			}
		}
	}
}

func syncFrame(ctx context.Context, corr *tsync.Correlator, f scan.Frame) {
	lg := slog.FromContext(ctx)
	switch f.Kind {
	case scan.NMEA:
		nmeaLog(lg, f.Data)
	case scan.UBX:
		u, err := ubx.ParseMsg(f.Data)
		if err != nil {
			lg.Error("ubxParseError", err)
		} else if u != nil {
			switch data := u.(type) {
			case *ubx.NavTimeGPS:
				corr.GPSTime(ptime.GPS(data.Week, data.ITOW), f.TRead)
			case *ubx.TimTP:
				corr.PulseCorrection(ptime.GPS(int16(data.Week), data.TowMS), Picoseconds(data.QErr))
			default:
				lg.Debug("ubx", "type", u.ID().String(), "payload", u)
			}
		}
	}
}

func Picoseconds(ps int32) time.Duration {
	if ps < 0 {
		return -Picoseconds(-ps)
	}
	return time.Duration(((ps + 500) / 1000))
}

const serReadTimeout = (time.Second * 11) / 10
const serMaxWriteLen = 4096

func serOpen(ctx context.Context, path string) (*term.Term, error) {
	t, err := term.Open(path, term.RawMode, term.FlowControl(term.NONE), term.ReadTimeout(serReadTimeout))
	if err != nil {
		return nil, err
	}
	err = t.Flush()
	if err != nil {
		t.Restore()
		t.Close()
		return nil, err
	}
	return t, nil
}

func serReadStart(ctx context.Context, scanner *scan.Scanner) chan scan.Frame {
	c := make(chan scan.Frame, 1) // XXX think about the buffering
	go serReadWorker(ctx, scanner, c)
	return c
}

func serReadWorker(ctx context.Context, p *scan.Scanner, c chan scan.Frame) {
	slog.FromContext(ctx).Debug("readWorkerStarted")
	defer close(c)
	for {
		f, err := p.Scan(ctx)
		c <- f
		if err != nil && err != io.EOF {
			if ctx.Err() == nil {
				slog.FromContext(ctx).Error("readError", err)
			}
			break
		}
	}
}

func serWriteAsync(ctx context.Context, w *term.Term, frames [][]byte) <-chan error {
	c := make(chan error, 1)
	go func() {
		for _, frame := range frames {
			select {
			case <-ctx.Done():
				c <- ctx.Err()
				return
			default:
			}
			_, err := serWrite(ctx, w, frame)
			if err != nil {
				c <- err
				return
			}
		}
		c <- serDrain(ctx, w)
		slog.FromContext(ctx).Debug("writeAsyncDone")
	}()
	return c
}

func serWrite(ctx context.Context, w *term.Term, buf []byte) (int, error) {
	total := 0
	lg := slog.FromContext(ctx)
	for len(buf) > 0 {

		// Semantics of Unix write and Go Write are not the same:
		// Unix can write less than requested amount without its being an error.
		wBuf := buf
		if len(buf) > serMaxWriteLen {
			wBuf = wBuf[0:serMaxWriteLen]
		}
		n, err := w.Write(wBuf)
		if err == io.ErrShortWrite && n > 0 {
			err = nil
		}
		if err == nil {
			lg.Debug("serialWrite", "n", n)
			total += n
			buf = buf[n:]
		} else if !errors.Is(err, unix.EINTR) {
			return total, err
		}
	}
	return total, nil
}

func serDrain(tx context.Context, w *term.Term) error {
	lg := slog.FromContext(tx)
	for {
		select {
		case <-tx.Done():
			return tx.Err()
		default:
		}
		n, err := w.Buffered()
		if err != nil || n == 0 {
			break
		}
		lg.Debug("serialBufferedBytes", "n", n)
		time.Sleep(time.Microsecond * 10)
		n, _ = w.Buffered()
		lg.Debug("drainBufferedBytes", "n", n)
	}
	return nil
}

const (
	serDevUnknown = iota
	serDevUART
	serDevUSB
	serDevUSBtoUART
	serDevBT
)

func serDevType(path string) int {
	s := unix.Stat_t{}
	err := unix.Stat(path, &s)
	if err != nil {
		return serDevUnknown
	}
	// See https://www.kernel.org/doc/html/latest/admin-guide/devices.html
	switch unix.Major(s.Dev) {
	case 4, 5:
		if unix.Minor(s.Dev) >= 64 { // ttyS0, /dev/ttycua0
			return serDevUART
		}
	case 166, 167: // USB ACM "modem" /dev/ttyACM0
		return serDevUSB
	case 188, 189: // USB serial converter /dev/ttyUSB0
		return serDevUSBtoUART
	case 204, 205: // low-density serial port (Raspberry Pi uses /dev/ttyAMA0)
		return serDevUART
	case 216, 217: // Bluetooth RFCOMM /dev/rfcomm0
		return serDevBT
	}
	return serDevUnknown
}
