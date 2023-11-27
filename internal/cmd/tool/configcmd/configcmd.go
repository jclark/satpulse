package configcmd

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/term"
)

func Cmd(lg *slog.Logger, progName string, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if v == nil {
		usage = usageFunc(progName)
		return
	}

	opts := gpsprot.ConfigOptions{Reset: v.reset, Flash: v.flash}

	var conn gpsio.Conn
	if v.serialDevice != "" {
		conn, err = gpsio.OpenSerial(v.serialDevice, v.localSpeed)
		opts.Detect = true
	} else {
		conn, err = gpsio.OpenSocket(v.socketPath)
	}
	if err != nil {
		return
	}
	cm := &gpsprot.ConfigMap{}

	if v.pps {
		cm.SetPPS()
	}
	if v.nmea {
		gpsprot.CfgNMEAEnabled.Set(cm, true)
	}
	if v.primaryGNSS != 0 {
		gpsprot.CfgPrimaryGNSS.Set(cm, v.primaryGNSS)
	}
	if v.enabledGNSS != 0 {
		gpsprot.CfgGNSSEnabled.Set(cm, v.enabledGNSS)
	}
	if v.remoteSpeed != 0 {
		if !term.IsValidSpeed(v.remoteSpeed) {
			err = fmt.Errorf("invalid remote serial speed %d", v.remoteSpeed)
			return
		}
		gpsprot.CfgBaudRate.Set(cm, uint32(v.remoteSpeed))
	}
	ctx := context.Background()
	ctx, _ = cmd.CancelOnSignal(ctx, lg)
	err = run(ctx, lg, cm, opts, conn)
	return
}

func run(ctx context.Context, lg *slog.Logger, cm *gpsprot.ConfigMap, opts gpsprot.ConfigOptions, conn gpsio.Conn) error {
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
