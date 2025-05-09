package gpscmd

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/term"
)

func Cmd(lg *slog.Logger, progName string, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if v == nil {
		usage = usageFunc(progName)
		return
	}

	target := gpsprot.NewConfigTarget(false)
	opts := &target.Opts
	opts.Reset = v.reset
	opts.Flash = v.flash
	opts.ForceProbe = v.force

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
	if v.enabledGNSS != 0 {
		cp.SetGNSSEnabled(v.enabledGNSS)
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
		// in gpsflags.go we only allow packet logging for serial connections
		conn.(*gpsio.SerialConn).SetOutPacketLogChan(logOutCh)
	}
	pCh := startScan(ctx, lg, &wg, conn, logCh)

	// Let the compiler check that TermError implements the SerialError interface
	// gpscfg relies on this
	var _ gpscfg.SerialError = gpsio.TermError{}
	info, err := gpscfg.Configure(ctx, lg, gpsreg.CreatePacketProcessors(nil), target, pCh, conn)
	if err == nil {
		fmt.Printf("set config to: %s\n", fmt.Sprint(info.ConfigProps))
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

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn, logCh chan<- scan.Packet) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	cmd.WaitGroupGo(wg, func() { gpsio.Scan(ctx, lg, conn, msg, logCh) })
	return msg
}
