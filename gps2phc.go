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
	clk   *phc.Clock
	term  *term.Term
	tsCh  <-chan phc.TsEvent
	gpsCh chan GpsMsg
	corr  *tsync.Correlator
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
	var s *Syncer
	if err == nil {
		s, err = newSyncer(ctx, clk)
	}
	if err == nil {
		for _, frame := range createConfig() {
			_, err = serWrite(ctx, s.term, frame)
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		slog.Error("exiting", err)
	} else {
		doSync(ctx, s)
		slog.Debug("exiting")
		err = s.term.Restore()
		if err != nil {
			slog.Error("could not restore terminal settings", err)
		}
		s.term.Close()
		slog.Debug("closed", "serial", serialDev)
		s.clk.Close()
		slog.Debug("closed", "if", ifName)
	}
}

func createConfig() [][]byte {
	return [][]byte{
		ubx.Poll[ubx.MonVer](),
		ubx.Poll[ubx.CfgTmode2](),
		ubx.Poll[ubx.CfgTp5](),
		ubx.Poll[ubx.TimSvin](),
		ubx.SetRate[ubx.NavTimeGPS](1),
		ubx.SetRate[ubx.TimTP](1),
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

func newSyncer(ctx context.Context, clk *phc.Clock) (r *Syncer, err error) {
	err = nil
	r = nil
	s := Syncer{clk: clk}
	lg := slog.FromContext(ctx)
	lg.Info("usingPHC", "path", clk.Path())
	s.tsCh, err = StartPPS(ctx, s.clk)
	if err != nil {
		return
	}
	/*
		// XXX errors after this need to deal with running PPS goroutine
		err = SkipStale(s.clk, s.tsCh)
		if err != nil {
			return nil, err
		}
	*/
	s.term, s.gpsCh, err = serStart(ctx, serialDev)
	if err != nil {
		return
	}
	servo, err := tsync.NewServo(s.clk, lg)
	if err != nil {
		return nil, err
	}
	s.corr = tsync.NewCorrelator(servo)
	r = &s
	return
}

func doSync(ctx context.Context, s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	gpsCh := s.gpsCh
	corr := s.corr
	lg := slog.FromContext(ctx)
	nSkipped := 0
	for tsCh != nil || gpsCh != nil {
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
		case g, ok := <-gpsCh:
			if ok {
				u := g.U
				switch data := u.(type) {
				case *ubx.NavTimeGPS:
					corr.GPSTime(ptime.GPS(data.Week, data.ITOW), g.TRead)
				case *ubx.TimTP:
					corr.PulseCorrection(ptime.GPS(int16(data.Week), data.TowMS), Picoseconds(data.QErr))
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
			} else {
				gpsCh = nil
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

func serStart(ctx context.Context, path string) (*term.Term, chan GpsMsg, error) {
	t, err := term.Open(path, term.RawMode, term.FlowControl(term.NONE), term.ReadTimeout(serReadTimeout))
	if err != nil {
		return nil, nil, err
	}
	err = t.Flush()
	if err != nil {
		return nil, nil, err
	}
	c := make(chan GpsMsg, 1)
	go serReadWorker(ctx, t, c)
	return t, c, nil
}

type GpsMsg struct {
	U     ubx.Msg
	TRead time.Time
}

func serReadWorker(ctx context.Context, r io.Reader, c chan GpsMsg) {
	defer close(c)
	p := scan.New(r, 16)
	lg := slog.FromContext(ctx)
	for {
		f, err := p.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				lg.Error("readError", err)
			}
			break
		}
		switch f.Kind {
		case scan.NMEA:
			lg.Debug("nmea", "type", string(f.Data[1:6]))
		case scan.UBX:
			ubxMsg, err := ubx.ParseMsg(f.Data)
			if err != nil {
				lg.Error("ubxParseError", err)
			} else if ubxMsg != nil {
				c <- GpsMsg{U: ubxMsg, TRead: f.TRead}
			}
		}
	}
}

func serWrite(ctx context.Context, w *term.Term, buf []byte) (int, error) {
	total := 0
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
			total += n
			buf = buf[n:]
		} else if !errors.Is(err, unix.EINTR) {
			return total, err
		}
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
	}
	return total, nil
}
