package gpscmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/ubx"
	"github.com/jclark/satpulse/term"
)

func Cmd(lg *slog.Logger, progName string, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if v == nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}

	target := gpsprot.NewConfigTarget(false)
	opts := &target.Opts
	opts.Reset = v.reset
	opts.Flash = v.flash
	opts.ForceProbe = v.forceProbe

	var conn gpsio.Conn
	if v.serialDevice != "" {
		conn, err = gpsio.OpenSerial(v.serialDevice, v.localSpeed)
	} else {
		conn, err = gpsio.OpenSocket(v.socketPath)
		opts.Detected = true
	}
	if err != nil {
		return
	}

	cp := &target.Props
	if v.pps {
		cp.SetPPS()
	}
	if v.nmea {
		cp.SetNMEAEnabled(true)
	}
	if v.primaryGNSS != 0 {
		cp.SetPrimaryGNSS(v.primaryGNSS)
	}
	if v.enabledSignals != 0 {
		cp.SetSignalsEnabled(v.enabledSignals)
	}
	if v.disableTimeMode {
		opts.Survey.When = 0
		cp.SetTimeMode(gpsprot.TimeModeDisabled)
	}
	if v.survey {
		opts.Survey.When = gpsprot.TimeModeAny
		opts.Survey.MinDur = time.Duration(v.surveyTime) * time.Second
		opts.Survey.AccLimit = gpsprot.Meters(v.surveyAcc)
	}
	if v.remoteSpeed != 0 {
		if !term.IsValidSpeed(v.remoteSpeed) {
			err = fmt.Errorf("invalid remote serial speed %d", v.remoteSpeed)
			return
		}
		cp.SetBaudRate(uint32(v.remoteSpeed))
	}
	if target.NoOp() {
		target.Get |= gpsprot.PropIDSignalsEnabled
	}
	ctx := context.Background()
	ctx, _ = cmd.CancelOnSignal(ctx, lg)
	err = run(ctx, lg, target, conn, v.packetLogPath)
	return
}

func run(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, conn gpsio.Conn, logPath string) error {
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

	logCh, logOutCh, err := gpsio.LogPackets(lg, &wg, logPath)
	if err != nil {
		return fmt.Errorf("failed to initialize packet logging: %w", err)
	}
	if logOutCh != nil {
		if serConn, ok := conn.(*gpsio.SerialConn); ok {
			serConn.SetOutPacketLogChan(logOutCh)
		} else {
			lg.Warn("logging output packets is not yet supported for socket connections")
			close(logOutCh)
		}
	}
	pCh := startScan(ctx, lg, &wg, conn, logCh)

	// Let the compiler check that TermError implements the SerialError interface
	// gpscfg relies on this
	var _ gpscfg.SerialError = gpsio.TermError{}
	rslt, err := gpscfg.Configure(ctx, lg, gpsreg.CreatePacketProcessors(nil), target, pCh, conn)
	if err == nil && rslt != nil {
		target.Get &^= gpsprot.PropIDSignalsEnabled
		if target.NoOp() {
			// print out the version only if we did not specify anything else
			printVersion(os.Stdout, rslt.Version)
		}
		// print out props that we know about (either requested or set)
		printProps(os.Stdout, rslt.ConfigProps)
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

func printVersion(f *os.File, v *ubx.Version) {
	if v == nil {
		return
	}
	if v.Mod != "" {
		fmt.Fprintf(f, "Model: %s\n", v.Mod)
	}
	if v.FW != nil {
		fmt.Fprintf(f, "Firmware version: %s\n", v.FW.String())
	}
	if v.Prot != nil {
		fmt.Fprintf(f, "UBX protocol version: %s\n", v.Prot.String())
	}
}

func printProps(f *os.File, p *gpsprot.ConfigProps) {
	if p == nil {
		return
	}
	if sigs, ok := p.GetSignalsEnabled(); ok {
		fmt.Fprintf(f, "Signals enabled: %s\n", sigs.String())
	}
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn, logCh chan<- scan.Packet) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	cmd.WaitGroupGo(wg, func() { gpsio.Scan(ctx, lg, conn, msg, logCh) })
	return msg
}
