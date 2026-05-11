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
	"net"
	"os"
	"strings"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/stream"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [--user user[:password]] [--bin] <address[:port]> <mountpoint>`

const defaultPort = "2101"

// scanBufSize matches stream.scanBufSize.
const scanBufSize = 16

type flagConfig struct {
	Addr       string
	Mountpoint string
	Username   string
	Password   string
	Bin        bool
}

func parseFlags(cmdName string, args []string) (cfg *flagConfig, help bool, usageFunc func(string) string, err error) {
	var user string
	var bin bool
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVar(&user, "user", "", "Ntrip credentials (password may be omitted)")
	flags.BoolVar(&bin, "bin", false, "emit raw bytes to stdout (default: packet log JSONL)")
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
		Addr:       ensurePort(flags.Arg(0), defaultPort),
		Mountpoint: flags.Arg(1),
		Bin:        bin,
	}
	cfg.Username, cfg.Password = splitUserPass(user)
	return
}

func ensurePort(addr, port string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, port)
}

func splitUserPass(user string) (string, string) {
	name, pass, _ := strings.Cut(user, ":")
	return name, pass
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
	ctx, _ := cmd.CancelOnSignal(context.Background(), lg)
	version, _ := cmd.Version()
	src := &stream.NtripSource{
		Addr:       cfg.Addr,
		Mountpoint: cfg.Mountpoint,
		Username:   cfg.Username,
		Password:   cfg.Password,
		UserAgent:  stream.NtripUserAgent{Version: version},
	}
	rc, err := src.Connect(ctx)
	if err != nil {
		return "", err
	}
	defer rc.Close()
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
