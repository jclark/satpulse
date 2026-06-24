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
	"os"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/stream"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/nmeamsg"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [--user user[:password]] [--bin] [--gga sentence] <address[:port]> <mountpoint>`

// scanBufSize matches stream.scanBufSize.
const scanBufSize = 16

type flagConfig struct {
	Addr       string
	Mountpoint string
	Username   string
	Password   string
	Bin        bool
	GGA        string // validated GGA sentence wire form (with CRLF), or empty
}

func parseFlags(cmdName string, args []string) (cfg *flagConfig, help bool, usageFunc func(string) string, err error) {
	var user string
	var bin bool
	var gga string
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVar(&user, "user", "", "Ntrip credentials (password may be omitted)")
	flags.BoolVar(&bin, "bin", false, "emit raw bytes to stdout (default: packet log JSONL)")
	flags.StringVar(&gga, "gga", "", "NMEA GGA sentence to send on connect (for VRS casters)")
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
	if gga != "" {
		if cfg.GGA, err = validateGGA(gga); err != nil {
			err = fmt.Errorf("--gga: %w", err)
			return
		}
	}
	return
}

func splitUserPass(user string) (string, string) {
	name, pass, _ := strings.Cut(user, ":")
	return name, pass
}

// validateGGA checks that s is a syntactically valid GGA sentence with a
// matching checksum, and returns its wire form with a CRLF appended.  The
// input is the sentence without a line terminator, e.g. "$GPGGA,...*47".
func validateGGA(s string) (string, error) {
	// Tolerate a trailing line terminator from copy-paste; the wire form
	// always ends in exactly one CRLF.
	s = strings.TrimRight(s, "\r\n")
	wire := s + "\r\n"
	if !nmeamsg.CheckSyntax(wire).IsValidApprovedNMEA() {
		return "", fmt.Errorf("not a valid NMEA sentence: %q", s)
	}
	// IsValidApprovedNMEA guarantees a 5-char address: "$" + 2-char talker
	// + 3-char sentence format, so the format is s[3:6].
	if s[3:6] != "GGA" {
		return "", fmt.Errorf("not a GGA sentence: %q", s)
	}
	payload, sum, _ := strings.Cut(s[1:], "*")
	want, _ := strconv.ParseUint(sum, 16, 8)
	if byte(want) != nmeamsg.Checksum([]byte(payload)) {
		return "", fmt.Errorf("checksum mismatch in %q", s)
	}
	return wire, nil
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
	src := &stream.NtripSource{
		Addr:       cfg.Addr,
		Mountpoint: cfg.Mountpoint,
		Username:   cfg.Username,
		Password:   cfg.Password,
		UserAgent:  stream.NtripUserAgent{Version: version},
	}
	var gs *stream.GGASender
	if cfg.GGA != "" {
		ch := make(chan scan.Packet, 1)
		ch <- scan.Packet{
			Format:        gpsreg.NMEAPacketFormat,
			Data:          cfg.GGA,
			ChecksumValid: true,
		}
		close(ch)
		gs = stream.NewGGASender(ch)
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
	formats := gpsreg.CreatePacketFormats(gpsreg.VendorUnknown)
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
