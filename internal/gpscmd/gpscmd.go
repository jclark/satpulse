package gpscmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/ubx"
)

func Cmd(lg *slog.Logger, progName string, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if v == nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}
	target, err := createConfigTarget(v)
	if err != nil {
		return
	}
	var conn gpsio.Conn
	if v.serialDevice != "" {
		conn, err = gpsio.OpenSerial(v.serialDevice, v.localSpeed)
	} else {
		conn, err = gpsio.OpenSocket(v.socketPath)
		target.Opts.Detected = true
	}
	if err != nil {
		return
	}
	ctx := context.Background()
	ctx, _ = cmd.CancelOnSignal(ctx, lg)
	err = run(ctx, lg, target, conn, v.packetLogPath, v.packetLogMode, args)
	return
}

func createConfigTarget(v *flagVars) (*gpsprot.ConfigTarget, error) {
	target := gpsprot.NewConfigTarget()

	target.Opts = v.configOpts

	cp := &target.Props
	if v.pps.IsSet() {
		cp.SetPPS(v.pps.Get())
	}
	if v.antCableDelay.IsSet() {
		cp.SetAntennaCableDelay(v.antCableDelay.Get())
	}
	if v.timeGNSS != 0 {
		cp.SetTimeGNSS(v.timeGNSS)
	}
	if v.enabledSignals != 0 {
		cp.SetSignalsEnabled(v.enabledSignals)
	}
	if v.mobile {
		cp.SetStationary(false)
	} else if !v.fixedPosECEF.IsZero() {
		cp.SetStationary(true)
	}
	if v.mode.IsSet() {
		cp.SetMode(v.mode.Get())
	}
	if v.navMsgAuth.IsSet() {
		cp.SetNavMsgAuth(v.navMsgAuth.Get())
	}
	if target.NoOp() {
		target.Get |= gpsprot.PropIDSignalsEnabled
	}
	return target, nil
}

func run(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, conn gpsio.Conn, logPath string, logMode packetLogMode, args []string) error {
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

	pktLog, lf, err := gpsio.LogPackets(lg, &wg, logPath)
	if err != nil {
		return fmt.Errorf("failed to initialize packet logging: %w", err)
	}
	if lf != nil {
		defer lf.Close(lg)
		if logMode == testLogMode {
			writeTestLogHead(lf, lg, args)
		}
	}
	if pktLog != nil {
		if serConn, ok := conn.(*gpsio.SerialConn); ok {
			serConn.SetPacketLog(pktLog)
		} else {
			lg.Warn("logging output packets is not yet supported for socket connections")
			pktLog.SemiClose() // Close needs to be called both for input and output packets
		}
	}
	pCh := startScan(ctx, lg, &wg, conn, pktLog)

	// Let the compiler check that TermError implements the SerialError interface
	// gpscfg relies on this
	var _ gpscfg.SerialError = gpsio.TermError{}
	rslt, err := gpscfg.Configure(ctx, lg, gpsreg.CreatePacketProcessors(nil), target, pCh, conn)
	if err == nil && rslt != nil {
		target.Get &^= gpsprot.PropIDSignalsEnabled
		if target.NoOp() {
			// print out the version only if we did not specify anything else
			printVersion(os.Stdout, rslt.Version)
		} else {
			logFailedProps(lg, &target.Props, rslt.ConfigProps)
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
	if logMode == testLogMode && lf != nil {
		writeTestLogTail(lf, lg, rslt, err)
	}
	return err
}

func logFailedProps(lg *slog.Logger, reqProps *gpsprot.ConfigProps, rsltProps *gpsprot.ConfigProps) {
	if reqProps == nil || rsltProps == nil {
		return
	}
	if reqSigs, ok := reqProps.GetSignalsEnabled(); ok {
		if rsltSigs, ok := rsltProps.GetSignalsEnabled(); ok {
			if reqSigs.GNSSSet() != rsltSigs.GNSSSet() {
				lg.Warn("only some of the requested constellations were enabled; the receiver does not support enabling all of them")
			}
		}
	}
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
	sigs, ok := p.GetSignalsEnabled()
	if !ok {
		return
	}
	groups := sigs.GNSSStringGroups()
	if len(groups) == 0 {
		return
	}
	constellations := make([]string, len(groups))
	for i, group := range groups {
		constellations[i] = group[0]
	}
	fmt.Fprintf(f, "Constellations enabled: %s\n", strings.Join(constellations, ", "))
	for _, group := range groups {
		fmt.Fprintf(f, "%s signals enabled: %s\n", group[0], strings.Join(group[1:], ", "))
	}
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn, pLog *gpsio.PacketLog) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	cmd.WaitGroupGo(wg, func() { gpsio.Scan(ctx, lg, conn, msg, pLog) })
	return msg
}
