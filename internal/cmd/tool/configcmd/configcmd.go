package configcmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/cmd/tool"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsmsg"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/term"

	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [-d|--serial-device path] [-s|--device-speed N] [--socket path]
            [--flash] [--reset] [--speed N] [--nmea] [-p|--pps] [-g|--gnss GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...]`

func Cmd(lg *slog.Logger, progName string, cmdName string, args []string) error {
	var help bool
	var pps bool
	var nmea bool
	var localSpeed cmd.IntFlag
	var remoteSpeed cmd.IntFlag
	var serialDevice string
	var socketPath string

	var gnss gnssList
	var opts gpsmsg.ConfigOptions

	cm := &gpsmsg.ConfigMap{}
	flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVar(&opts.Flash, "flash", false, "save the configuration changes to flash memory on the GNSS receiver")
	flags.BoolVar(&opts.Reset, "reset", false, "reset the GNSS receiver")
	flags.BoolVar(&nmea, "nmea", false, "enable NMEA output from the GNSS receiver")
	flags.StringVarP(&serialDevice, "serial-device", "d", "", "serial device to configure")
	flags.StringVar(&socketPath, "socket", "", "`path` of socket to connect to GPS")
	flags.VarP(&localSpeed, "device-speed", "s", "serial device baud-rate in `bps`")
	flags.Var(&remoteSpeed, "speed", "set GNSS receiver baud-rate in `bps`")
	flags.VarP(&gnss, "gnss", "g", "set `list` of enabled GNSS constellations: GPS|GAL|BDS|GLO|QZSS|NAVIC|SBAS,...")
	flags.BoolVarP(&pps, "pps", "p", false, "configure the receiver to enable a PPS signal")
	err := flags.Parse(args)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(2)
	}
	if help {
		tool.Usage(progName, cmdName, summary, flags)
		os.Exit(0)
	}
	if flags.NArg() != 0 {
		cmd.ErrPrintln(progName, "config command must not have arguments")
		tool.Usage(progName, cmdName, summary, flags)
		os.Exit(2)
	}
	if (socketPath == "") == (serialDevice == "") {
		cmd.ErrPrintln(progName, "config command must specify either --socket or --serial-device")
		tool.Usage(progName, cmdName, summary, flags)
		os.Exit(2)
	}
	var conn gpsio.Conn
	if serialDevice != "" {
		conn, err = gpsio.OpenSerial(serialDevice, localSpeed.Value)
		opts.Detect = true
	} else {
		conn, err = gpsio.OpenSocket(socketPath)
	}
	if err != nil {
		return err
	}
	if pps {
		cm.SetPPS()
	}
	if nmea {
		gpsmsg.CfgNMEAEnabled.Set(cm, true)
	}
	if len(gnss.gnss) != 0 {
		first := gnss.gnss[0]
		if !first.IsMajor() {
			return fmt.Errorf("first GNSS must be a major constellation: %s is not", first)
		}
		gpsmsg.CfgPrimaryGNSS.Set(cm, gnss.gnss[0])
		gpsmsg.CfgGNSSEnabled.Set(cm, gnss.GNSSSet())
	}
	if p := remoteSpeed.Value; p != nil {
		n := *p
		if !term.IsValidSpeed(n) {
			return fmt.Errorf("invalid remote serial speed %d", n)
		}
		gpsmsg.CfgBaudRate.Set(cm, uint32(n))
	}
	ctx := context.Background()
	ctx, _ = cmd.CancelOnSignal(ctx, lg)
	return run(ctx, lg, cm, opts, conn)
}

func run(ctx context.Context, lg *slog.Logger, cm *gpsmsg.ConfigMap, opts gpsmsg.ConfigOptions, conn gpsio.Conn) error {
	defer func() {
		addr := conn.LocalAddr()
		lg.Debug("closing the GPS connection", "addr", addr)
		e := conn.Close()
		if e != nil {
			lg.Error("error closing the GPS connection", "addr", addr, "error", e)
		} else {
			lg.Debug("successfully closed the GPS connection", "addr", addr)
		}
	}()

	var wg sync.WaitGroup

	pCh := startScan(ctx, lg, &wg, conn)

	// Let the compiler check that TermError implements the SerialError interface
	// gpscfg relies on this
	var _ gpscfg.SerialError = gpsio.TermError{}
	info, err := gpscfg.Configure(ctx, lg, cm, opts, pCh, conn)
	if err == nil {
		fmt.Printf("set config to: %s\n", fmt.Sprint(info.ConfigMap))
	}

	lg.Debug("about to wait")

	// stop the scanner
	conn.Stop()
	// need to keep reading here to avoid deadlock
	for range pCh {
	}
	wg.Wait()
	return err
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	cmd.WaitGroupGo(wg, func() { gpsio.Scan(ctx, lg, conn, msg) })
	return msg
}

type gnssList struct {
	gnss []gpsmsg.GNSS
}

var _ pflag.Value = (*gnssList)(nil)

func (gl *gnssList) String() string {
	var s []string
	for _, gnss := range gl.gnss {
		s = append(s, gnss.String())
	}
	return strings.Join(s, ",")
}

func (gl *gnssList) Type() string {
	return "gnss-list"
}

func (gl *gnssList) GNSSSet() gpsmsg.GNSSSet {
	var flags gpsmsg.GNSSSet
	for _, g := range gl.gnss {
		flags |= gpsmsg.GNSSFlag(g)
	}
	return flags
}

func (gl *gnssList) Set(s string) error {
	words := strings.Split(s, ",")
	for _, w := range words {
		gnss, err := gpsmsg.ParseGNSS(strings.Trim(w, " \t"))
		if err != nil {
			return err
		}
		gl.gnss = append(gl.gnss, gnss)
	}
	return nil
}
