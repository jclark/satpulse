package gpscmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
)

func Cmd(logWriter io.Writer, logLevel slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
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
	var msgs any
	if v.msgFilePath != "" {
		var mf *MsgFile
		mf, err = LoadMsgFile(v.msgFilePath)
		if err != nil {
			return
		}
		// Check for inconsistent tag descriptions regardless of showTags.
		tds := mf.CheckTagDescriptions(lg)
		if v.showTags {
			printTagDescs(os.Stderr, tds)
			return
		}
		msgs, err = mf.TaggedMsgs(v.msgTags)
		if err != nil {
			return
		}
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
	err = run(ctx, lg, target, msgs, conn, v.packetLogPath, v.packetLogMode, v.capture, v.showReceiver, args)
	return
}

// createConfigTarget returns nil if msgFilePath is set (message file mode)
// or if nothing requires the configurator (passive capture mode).
func createConfigTarget(v *flagVars) (*gpsprot.ConfigTarget, error) {
	if v.msgFilePath != "" {
		return nil, nil
	}
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
	// If nothing requires the configurator, return nil (passive capture mode)
	if !v.showReceiver && target.NoOp() {
		return nil, nil
	}
	if v.socketPath != "" {
		target.Opts.Socket = true
	}
	if target.NoOp() {
		target.Opts.ForceProbe = true
	}
	return target, nil
}

func configTargetIsProbeOnly(target *gpsprot.ConfigTarget) bool {
	if target.NoOp() {
		return false
	}
	copy := *target
	copy.Opts.ForceProbe = false
	copy.Opts.Socket = false
	return copy.NoOp()
}

// run executes the GPS command.
//
// Modes based on target and msgs:
//   - target non-nil: config mode (runs GPS configuration)
//   - msgs non-nil: message file mode (sends user-defined messages)
//   - both nil: passive capture mode (just logs packets, no interaction)
//
// Parameter dependencies:
//   - logMode: must not be testLogMode when msgs is non-nil
//   - args: only used for test log header when logMode is testLogMode
func run(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, msgs any, conn gpsio.Conn, logPath string, logMode packetLogMode, capture gpsprot.Option[time.Duration], showReceiver bool, args []string) error {
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

	var rslt *gpscfg.Result
	if msgs != nil {
		err = runMsgs(ctx, lg, conn, pCh, msgs, capture)
	} else if target != nil {
		rslt, err = runConfig(ctx, lg, target, pCh, conn, capture, showReceiver)
	} else {
		// Passive capture mode: just read and log packets
		if capture.IsSet() {
			keepReading(ctx, lg, pCh, capture.Get(), nil)
		}
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

func runConfig(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, pCh <-chan scan.Packet, conn gpsio.Conn, capture gpsprot.Option[time.Duration], showReceiver bool) (*gpscfg.Result, error) {
	// Let the compiler check that TermError implements the SerialError interface
	// gpscfg relies on this
	var _ gpscfg.SerialError = gpsio.TermError{}
	rslt, err := gpscfg.Configure(ctx, lg, gpsreg.CreatePacketProcessors(nil), gpsreg.CreateConfigProtocols(), target, pCh, conn)
	if errors.Is(err, gpscfg.ErrNoProbeResponse) && configTargetIsProbeOnly(target) {
		err = nil
	}
	if err == nil && rslt != nil {
		if showReceiver {
			printReceiverInfo(os.Stdout, rslt.ReceiverInfo)
			printPacketFormats(os.Stdout, rslt.PacketFormatsDetected)
		}
		if !configTargetIsProbeOnly(target) {
			logFailedProps(lg, &target.Props, rslt.ConfigProps)
		}
		printProps(os.Stdout, rslt.ConfigProps)
	}
	if capture.IsSet() {
		keepReading(ctx, lg, pCh, capture.Get(), nil)
	}
	return rslt, err
}

func runMsgs(ctx context.Context, lg *slog.Logger, conn gpsio.Conn, pCh <-chan scan.Packet, msgs any, capture gpsprot.Option[time.Duration]) error {
	var rp *responsePrinter
	var err error
	var raw []rawMsg
	switch m := msgs.(type) {
	case []LineMsg:
		raw, err = toRawMsgs(m)
		if err != nil {
			return err
		}
		rp = newResponsePrinter(os.Stdout)
	case []BinaryMsg:
		raw, err = toRawMsgs(m)
		if err != nil {
			return err
		}
	case []NMEAMsg:
		raw, err = toRawMsgs(m)
		if err != nil {
			return err
		}
		rp = newResponsePrinter(os.Stdout)
	case []CASBINMsg:
		raw, err = toRawMsgs(m)
		if err != nil {
			return err
		}
		rp = newResponsePrinter(os.Stdout)
	case []ASBINMsg:
		raw, err = toRawMsgs(m)
		if err != nil {
			return err
		}
		rp = newResponsePrinter(os.Stdout)
	case []UBXMsg:
		raw, err = toRawMsgs(m)
		if err != nil {
			return err
		}
		rp = newResponsePrinter(os.Stdout)
	default:
		panic(fmt.Sprintf("unexpected message type: %T", msgs))
	}
	err = runRawMsgs(ctx, lg, conn, pCh, raw, rp)
	if capture.IsSet() {
		keepReading(ctx, lg, pCh, capture.Get(), rp)
	}
	if rp != nil {
		rp.Flush()
	}
	return err
}

func runRawMsgs(ctx context.Context, lg *slog.Logger, conn gpsio.Conn, pCh <-chan scan.Packet, msgs []rawMsg, rp *responsePrinter) error {
	for _, m := range msgs {
		if err := sendMsg(ctx, lg, conn, pCh, m, rp); err != nil {
			return err
		}
	}
	return nil
}

func sendMsg(ctx context.Context, lg *slog.Logger, conn gpsio.Conn, pCh <-chan scan.Packet, m rawMsg, rp *responsePrinter) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	_, err := conn.Write(m.bytes)
	if err != nil {
		return err
	}
	lg.Info("sent message", "index", m.index+1, "tag", m.tag)
	var timerCh <-chan time.Time
	if m.delay > 0 {
		timer := time.NewTimer(m.delay)
		defer timer.Stop()
		timerCh = timer.C
	}
	for timerCh != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timerCh:
			timerCh = nil
		case pkt, ok := <-pCh:
			if !ok {
				return nil
			}
			if rp != nil {
				rp.handlePacket(pkt)
			}
		}
	}

	// Drain any immediately available packets (including when delay is zero).
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case pkt, ok := <-pCh:
			if !ok {
				return nil
			}
			if rp != nil {
				rp.handlePacket(pkt)
			}
		default:
			return nil
		}
	}
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

func keepReading(ctx context.Context, lg *slog.Logger, pCh <-chan scan.Packet, dur time.Duration, rp *responsePrinter) {
	if dur == 0 {
		lg.Info("capturing packets until interrupted")
	} else {
		lg.Debug("capturing packets", "duration", dur)
	}
	var timerC <-chan time.Time
	if dur > 0 {
		timer := time.NewTimer(dur)
		defer timer.Stop()
		timerC = timer.C
	}
	for {
		select {
		case <-ctx.Done():
			lg.Debug("capture interrupted")
			return
		case <-timerC:
			lg.Debug("capture complete")
			return
		case pkt, ok := <-pCh:
			if !ok {
				return
			}
			if rp != nil {
				rp.handlePacket(pkt)
			}
		}
	}
}
