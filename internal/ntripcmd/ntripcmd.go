// Package ntripcmd implements the satpulsetool ntrip subcommand, which
// fetches data from an Ntrip caster and writes either a JSONL packet log or
// raw bytes to stdout.
package ntripcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/stream"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [--user user[:password]] [--bin] [--nmea-send-pos lat,lon[,hgt]] [--nmea-send-interval secs] <address[:port]> <mountpoint>`

const (
	// scanBufSize matches stream.scanBufSize.
	scanBufSize = 16

	nmeaSendPosTalker         = "GN"
	nmeaSendPosQuality        = uint8(1)
	defaultNMEASendPosNumSats = uint8(12)
	defaultNMEASendPosHDOP    = 1.0
)

type nmeaSendPos struct {
	LatLon [2]float64
	Height float64
}

type flagConfig struct {
	Addr             string
	Mountpoint       string
	Username         string
	Password         string
	Bin              bool
	NMEASendPos      *nmeaSendPos
	NMEASendInterval time.Duration
}

func parseFlags(cmdName string, args []string) (cfg *flagConfig, help bool, usageFunc func(string) string, err error) {
	var user string
	var bin bool
	var sendPos string
	sendInterval := stream.DefaultNMEASendInterval.Seconds()
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVar(&user, "user", "", "Ntrip credentials (password may be omitted)")
	flags.BoolVar(&bin, "bin", false, "emit raw bytes to stdout (default: packet log JSONL)")
	flags.StringVar(&sendPos, "nmea-send-pos", "", "receiver position as lat,lon[,hgt] for VRS casters")
	flags.Float64Var(&sendInterval, "nmea-send-interval", sendInterval, "seconds between GGA re-sends (0 = once); requires --nmea-send-pos")
	usageFunc = cmd.UsageFunc(cmdName, summary, flags)
	if err = flags.Parse(args); err != nil {
		return
	}
	if help {
		return
	}
	if flags.NArg() != 2 {
		err = fmt.Errorf("expected two positional arguments: <address[:port]> <mountpoint>")
		return
	}
	cfg = &flagConfig{
		Addr:       stream.NtripNormalizeAddress(flags.Arg(0)),
		Mountpoint: flags.Arg(1),
		Bin:        bin,
	}
	cfg.Username, cfg.Password = splitUserPass(user)
	if flags.Changed("nmea-send-pos") {
		if cfg.NMEASendPos, err = parseNMEASendPos(sendPos); err != nil {
			err = fmt.Errorf("--nmea-send-pos: %w", err)
			return
		}
	}
	// Unlike the config file, where nmeaSendInterval is validated but accepted
	// regardless of nmeaSend, the CLI has no position to send without
	// --nmea-send-pos, so an interval alone is a user mistake worth flagging.
	if flags.Changed("nmea-send-interval") && !flags.Changed("nmea-send-pos") {
		err = fmt.Errorf("--nmea-send-interval requires --nmea-send-pos")
		return
	}
	if sendInterval < 0 || math.IsNaN(sendInterval) || math.IsInf(sendInterval, 0) {
		err = fmt.Errorf("--nmea-send-interval: must be a non-negative finite number")
		return
	}
	if sendInterval > 0 && sendInterval < stream.MinNMEASendInterval.Seconds() {
		err = fmt.Errorf("--nmea-send-interval: must be 0 or at least 1")
		return
	}
	if sendInterval > stream.MaxNMEASendInterval.Seconds() {
		err = fmt.Errorf("--nmea-send-interval: must be at most %g", stream.MaxNMEASendInterval.Seconds())
		return
	}
	cfg.NMEASendInterval = time.Duration(sendInterval * float64(time.Second))
	return
}

func splitUserPass(user string) (string, string) {
	name, pass, _ := strings.Cut(user, ":")
	return name, pass
}

func parseNMEASendPos(s string) (*nmeaSendPos, error) {
	fields := strings.Split(s, ",")
	if len(fields) != 2 && len(fields) != 3 {
		return nil, fmt.Errorf("expected lat,lon[,hgt]")
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	lat, err := parseFiniteFloat("lat", fields[0])
	if err != nil {
		return nil, err
	}
	lon, err := parseFiniteFloat("lon", fields[1])
	if err != nil {
		return nil, err
	}
	h := 0.0
	if len(fields) == 3 {
		if h, err = parseFiniteFloat("hgt", fields[2]); err != nil {
			return nil, err
		}
	}
	if lat < -90 || lat > 90 {
		return nil, fmt.Errorf("lat out of range: %v", lat)
	}
	if lon < -180 || lon > 180 {
		return nil, fmt.Errorf("lon out of range: %v", lon)
	}
	return &nmeaSendPos{LatLon: [2]float64{lat, lon}, Height: h}, nil
}

func parseFiniteFloat(name, s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("%s is empty", name)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("%s is not finite: %v", name, v)
	}
	return v, nil
}

func makeNMEASendPosGGA(t time.Time, pos *nmeaSendPos) (string, error) {
	if pos == nil {
		return "", nil
	}
	h := pos.Height
	n := defaultNMEASendPosNumSats
	hdop := defaultNMEASendPosHDOP
	m := nmeamsg.MakeGGA(nmeaSendPosTalker, t, &pos.LatLon, &h, &h, nmeaSendPosQuality, &n, &hdop)
	b, err := nmeamsg.SerializeMsg(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Cmd implements the ntrip subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
	cfg, help, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	ctx, cancel := cmd.CancelOnSignal(context.Background(), lg)
	defer cancel()
	version, _ := cmd.Version()
	gga, err := makeNMEASendPosGGA(time.Now(), cfg.NMEASendPos)
	if err != nil {
		return "", err
	}
	src := &stream.NtripSource{
		Addr:       cfg.Addr,
		Mountpoint: cfg.Mountpoint,
		Username:   cfg.Username,
		Password:   cfg.Password,
		UserAgent:  stream.NtripUserAgent{Version: version},
	}
	var gs *stream.GGASender
	if gga != "" {
		ch := make(chan scan.Packet, 1)
		ch <- scan.Packet{
			Format:        gpsreg.NMEAPacketFormat,
			Data:          gga,
			ChecksumValid: true,
		}
		close(ch)
		gs = stream.NewGGASender(ch, cfg.NMEASendInterval)
		go gs.Run(ctx, lg)
		if err := gs.WaitReady(ctx); err != nil {
			return "", err
		}
	}
	rc, err := src.Connect(ctx)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	if gs != nil && !gs.SetWriter(ctx, rc) {
		return "", ctx.Err()
	}
	// Close rc on ctx cancellation so a blocked Read returns.
	// NtripSource only watches ctx during the handshake.
	go func() {
		<-ctx.Done()
		rc.Close()
	}()
	if cfg.Bin {
		_, err = io.Copy(os.Stdout, rc)
		return "", cleanExitErr(ctx, err)
	}
	return "", scanLoop(ctx, rc, os.Stdout)
}

func scanLoop(ctx context.Context, r io.Reader, w io.Writer) error {
	formats := gpsreg.CreateCorrectionFormats()
	scanner := scan.New(r, scanBufSize, formats)
	enc := json.NewEncoder(w)
	for {
		pkt, err := scanner.Scan()
		if err != nil {
			return cleanExitErr(ctx, err)
		}
		if len(pkt.Data) == 0 {
			continue
		}
		if err := enc.Encode(gpsio.MakePacketLogEntry(pkt)); err != nil {
			return err
		}
	}
}

// cleanExitErr returns nil if err is io.EOF or ctx has been cancelled,
// otherwise err.
func cleanExitErr(ctx context.Context, err error) error {
	if err == nil || errors.Is(err, io.EOF) || ctx.Err() != nil {
		return nil
	}
	return err
}
