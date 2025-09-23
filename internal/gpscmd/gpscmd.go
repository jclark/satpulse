package gpscmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/scan"
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
	target.Get = v.configGet

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
	if v.mode.IsSet() {
		cp.SetMode(v.mode.Get())
	}
	if v.navMsgAuth.IsSet() {
		cp.SetNavMsgAuth(v.navMsgAuth.Get())
	}
	if v.socketPath != "" {
		target.Opts.Detected = true
	}
	if target.NoOp() {
		target.Opts.ForceProbe |= gpsprot.ForceProbeWhenNoConfig
	}
	return target, nil
}

func configTargetIsProbeOnly(target *gpsprot.ConfigTarget) bool {
	if target.NoOp() {
		return false
	}
	copy := *target
	copy.Opts.ForceProbe &^= gpsprot.ForceProbeWhenNoConfig
	copy.Opts.Detected = false
	return copy.NoOp()
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
	rslt, err := gpscfg.Configure(ctx, lg, gpsreg.CreatePacketProcessors(nil), gpsreg.CreateConfigProtocols(), target, pCh, conn)
	if errors.Is(err, gpscfg.ErrNoProbeResponse) && configTargetIsProbeOnly(target) {
		err = nil
	}
	if err == nil && rslt != nil {
		if configTargetIsProbeOnly(target) {
			// print out the version only if we did not specify anything else
			printReceiverInfo(os.Stdout, rslt.ReceiverInfo)
			printPacketFormats(os.Stdout, rslt.PacketFormatsDetected)
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

func printReceiverInfo(f *os.File, info *gpsprot.ReceiverInfo) {
	if info == nil {
		return
	}
	if info.Vendor != "" {
		fmt.Fprintf(f, "Vendor: %s\n", info.Vendor)
	}
	if info.Hardware != "" {
		fmt.Fprintf(f, "Hardware: %s\n", info.Hardware)
	}
	if info.Firmware != "" {
		fmt.Fprintf(f, "Firmware: %s\n", info.Firmware)
	}
	if info.SupportedGNSS != 0 {
		fmt.Fprintf(f, "Supported GNSS: %s\n", info.SupportedGNSS)
	}
}

func printPacketFormats(f *os.File, tags []gpsprot.Tag) {
	if len(tags) == 0 {
		return
	}
	formats := make([]string, len(tags))
	for i, tag := range tags {
		formats[i] = string(tag)
	}
	fmt.Fprintf(f, "Packet formats detected: %s\n", strings.Join(formats, ", "))
}

func printProps(f *os.File, p *gpsprot.ConfigProps) {
	if p == nil {
		return
	}
	if sigs, ok := p.GetSignalsEnabled(); ok {
		printSignals(f, sigs)
	}
	if timeGNSS, ok := p.GetTimeGNSS(); ok {
		printTimeGNSS(f, timeGNSS)
	}
	if antCableDelay, ok := p.GetAntennaCableDelay(); ok {
		printAntennaCableDelay(f, antCableDelay)
	}
	if timePulse, ok := p.GetTimePulse(); ok {
		printTimePulse(f, timePulse)
	}
	if mode, ok := p.GetMode(); ok {
		printMode(f, mode)
	}
}

func printSignals(f *os.File, sigs gpsprot.SignalSet) {
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

func printMode(f *os.File, mode gpsprot.Mode) {
	modeName := "mobile"
	if mode.Static {
		modeName = "static"
	}
	fmt.Fprintf(f, "Mode: %s\n", modeName)
	if !mode.Static {
		return
	}
	switch mode.PosType {
	case gpsprot.PosTypeNone:
		return
	case gpsprot.PosTypeECEF:
		fmt.Fprintf(f, "Fixed position ECEF: %s\n", mode.FixedPosECEF.String())
	}
	fmt.Fprintf(f, "Fixed position accuracy: %s m\n", mode.FixedPosAcc.String())
}

func printTimeGNSS(f *os.File, timeGNSS gpsprot.GNSS) {
	fmt.Fprintf(f, "Time GNSS: %s\n", timeGNSS.String())
}

func printAntennaCableDelay(f *os.File, delay time.Duration) {
	fmt.Fprintf(f, "Antenna cable delay: %d ns\n", delay.Nanoseconds())
}

func printTimePulse(f *os.File, tp gpsprot.TimePulse) {
	if tp.Width == 0 {
		fmt.Fprint(f, "Time pulse: disabled\n")
		return
	}
	polarity := "falling"
	if tp.PolarityRising {
		polarity = "rising"
	}
	flags := ""
	if tp.AlignToGNSS {
		flags = "; aligned to GNSS time"
	}
	if tp.OnlyWhenLocked {
		flags += "; only when locked"
	}
	fmt.Fprintf(f, "Time pulse: enabled; width %g s; period %g s; polarity %s%s\n", tp.Width.Seconds(), tp.Period.Seconds(), polarity, flags)
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn, pLog *gpsio.PacketLog) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(ctx, lg, conn, msg, pLog) })
	return msg
}
