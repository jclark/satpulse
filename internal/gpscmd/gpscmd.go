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
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/msgfile"
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
	var raw []msgfile.RawMsg
	if v.msgFilePath != "" {
		var mf *msgfile.Parsed
		mf, err = msgfile.Load(v.msgFilePath)
		if err != nil {
			return
		}
		tds, inconsistent := mf.TagDescs()
		for _, td := range inconsistent {
			lg.Warn("tag has inconsistent descriptions", "tag", td.Tag, "desc", td.Desc)
		}
		if v.showTags {
			if err = mf.ValidateTags(); err != nil {
				return
			}
			msgfile.PrintTagDescs(os.Stdout, tds)
			return
		}
		var msgs any
		msgs, err = mf.TaggedMsgs(v.msgTags)
		if err != nil {
			return
		}
		// Convert to raw bytes before opening the GPS connection so that
		// configuration errors (including --save against non-save-aware
		// messages, or ubxvalport without --port) surface without first
		// touching the device.
		raw, err = msgfile.ToRaw(msgs, v.msgPort, v.msgSave)
		if err != nil {
			return
		}
	}
	var conn gpsio.Conn
	if v.serialDevice != "" {
		conn, _, err = gpsio.OpenSerial(v.serialDevice, v.localSpeed)
	} else {
		conn, err = gpsio.OpenSocket(v.socketPath)
	}
	if err != nil {
		return
	}
	ctx := context.Background()
	ctx, _ = cmd.CancelOnSignal(ctx, lg)
	err = run(ctx, lg, target, raw, conn, v.vendor, v.packetLogPath, v.packetLogMode, v.capture, v.showReceiver, v.jsonOut, v.configSupport, args)
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
	if v.minElev.IsSet() {
		cp.SetMinElevation(v.minElev.Get())
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
	if v.rtcmBaseID.IsSet() {
		cp.SetRTCMBaseID(v.rtcmBaseID.Get())
	}
	if v.baudRate.IsSet() {
		cp.SetBaudRate(v.baudRate.Get())
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
// Modes based on target and raw:
//   - target non-nil: config mode (runs GPS configuration)
//   - raw non-nil: message file mode (sends user-defined messages)
//   - both nil: passive capture mode (just logs packets, no interaction)
//
// Parameter dependencies:
//   - logMode: must not be testLogMode when raw is non-nil
//   - args: only used for test log header when logMode is testLogMode
func run(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, raw []msgfile.RawMsg, conn gpsio.Conn, vendor gpsreg.Vendor, logPath string, logMode packetLogMode, capture opt.Val[time.Duration], showReceiver bool, jsonOut bool, support configSupportReq, args []string) error {
	defer func() {
		addr := conn.LocalAddr()
		lg.Debug("closing the GPS connection", "addr", addr)
		// Drain first so a final no-response command (e.g. a reset) reaches
		// the receiver before Close restores and closes the port.
		if e := conn.Drain(); e != nil {
			lg.Debug("error draining the GPS connection before close", "addr", addr, "error", e)
		}
		e := conn.Close()
		if e != nil {
			lg.Error("error closing the GPS connection", "addr", addr, "error", e)
		} else {
			lg.Debug("successfully closed the GPS connection", "addr", addr)
		}
	}()

	var wg sync.WaitGroup

	pktFormats := gpsreg.CreatePacketFormats(vendor)
	pktLog, lf, err := gpsio.LogPackets(lg, &wg, logPath, false, pktFormats)
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
	pCh := startScan(ctx, lg, &wg, conn, pktLog, pktFormats)

	var rslt *gpscfg.Result
	if raw != nil {
		err = runMsgs(ctx, lg, conn, pCh, raw, capture)
	} else if target != nil {
		rslt, err = runConfig(ctx, lg, target, pCh, conn, vendor, capture, showReceiver, jsonOut, support)
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

func runConfig(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, pCh <-chan scan.Packet, conn gpsio.Conn, vendor gpsreg.Vendor, capture opt.Val[time.Duration], showReceiver bool, jsonOut bool, support configSupportReq) (*gpscfg.Result, error) {
	// Compile-time check: serial faults surfaced by gpsio satisfy the
	// gpscfg.SerialError interface. gpscfg relies on this.
	var _ gpscfg.SerialError = (*gpsio.SerialError)(nil)
	pktProcs := gpsreg.CreatePacketProcessors(vendor)
	gpsprot.SetAllMsgHandlers(pktProcs, &gpsprot.DefaultHandler{})
	rslt, err := gpscfg.Configure(ctx, lg, pktProcs, gpsreg.CreateConfigProtocols(vendor), target, pCh, conn)
	if errors.Is(err, gpscfg.ErrNoProbeResponse) && configTargetIsProbeOnly(target) {
		err = nil
	}
	if err == nil && rslt != nil {
		warnMissingConfigSupport(lg, support, rslt.ConfigSupport)
		if !configTargetIsProbeOnly(target) {
			logFailedProps(lg, &target.Props, rslt.ConfigProps)
		}
	}
	if jsonOut {
		printJSON(os.Stdout, rslt, err)
	} else if err == nil && rslt != nil {
		if showReceiver {
			printReceiverInfo(os.Stdout, rslt.ReceiverInfo)
			printConfigSupport(os.Stdout, rslt.ConfigSupport)
			printPacketFormats(os.Stdout, rslt.PacketFormatsDetected)
		}
		printProps(os.Stdout, rslt.ConfigProps)
	}
	if capture.IsSet() {
		keepReading(ctx, lg, pCh, capture.Get(), nil)
	}
	return rslt, err
}

func warnMissingConfigSupport(lg *slog.Logger, req configSupportReq, supported gpsprot.ConfigSupportFlags) {
	if req.isZero() {
		return
	}
	for _, opt := range req.unsupportedOptions(supported) {
		lg.Warn("receiver does not support the following option", "option", opt)
	}
}

func runMsgs(ctx context.Context, lg *slog.Logger, conn gpsio.Conn, pCh <-chan scan.Packet, raw []msgfile.RawMsg, capture opt.Val[time.Duration]) error {
	rh := newResponseHandler(os.Stdout, lg)
	err := sendAllMsgs(ctx, lg, conn, pCh, raw, rh)
	if capture.IsSet() {
		keepReading(ctx, lg, pCh, capture.Get(), rh)
	}
	rh.reportMissing()
	rh.Flush()
	return err
}

func sendAllMsgs(ctx context.Context, lg *slog.Logger, conn gpsio.Conn, pCh <-chan scan.Packet, msgs []msgfile.RawMsg, rh *responseHandler) error {
	var deadline time.Time
	for i, m := range msgs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := conn.Write(m.Bytes)
		if err != nil {
			return err
		}
		rh.notifySent(m)
		lg.Info("sent message", "index", m.Index+1, "tag", m.Tag)
		if d := time.Now().Add(m.WaitLimit); d.After(deadline) {
			deadline = d
		}
		if err := waitAfterSend(ctx, lg, pCh, m, msgs, i, deadline, rh); err != nil {
			return err
		}
	}
	// Wait for remaining responses until deadline.
	return waitForResponses(ctx, lg, pCh, deadline, rh)
}

// waitAfterSend waits for the message's delay, then for pacing to
// clear for the next message. Packets are processed throughout.
func waitAfterSend(ctx context.Context, lg *slog.Logger, pCh <-chan scan.Packet, m msgfile.RawMsg, msgs []msgfile.RawMsg, i int, deadline time.Time, rh *responseHandler) error {
	last := i == len(msgs)-1
	// Wait for delay.
	if m.Delay > 0 {
		timer := time.NewTimer(m.Delay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				goto delayDone
			case pkt, ok := <-pCh:
				if !ok {
					return nil
				}
				rh.handlePacket(pkt)
			}
		}
	}
delayDone:
	if last {
		return nil
	}
	// Wait for pacing: ReadyToSend for the next message.
	next := msgs[i+1]
	if rh.readyToSend(next) {
		return nil
	}
	lg.Debug("waiting for pending ACK before sending", "index", next.Index+1, "tag", next.Tag)
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		case pkt, ok := <-pCh:
			if !ok {
				return nil
			}
			rh.handlePacket(pkt)
			if rh.readyToSend(next) {
				return nil
			}
		}
	}
}

// waitForResponses processes packets until all responses are received
// or the deadline expires.
func waitForResponses(ctx context.Context, lg *slog.Logger, pCh <-chan scan.Packet, deadline time.Time, rh *responseHandler) error {
	if !rh.canAcceptMore() {
		return nil
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil
	}
	lg.Debug("waiting for responses", "remaining", remaining)
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		case pkt, ok := <-pCh:
			if !ok {
				return nil
			}
			rh.handlePacket(pkt)
			if !rh.canAcceptMore() {
				lg.Debug("all responses received")
				return nil
			}
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

func printConfigSupport(f *os.File, support gpsprot.ConfigSupportFlags) {
	if support != 0 {
		fmt.Fprintf(f, "Supports: %s\n", support)
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
	if minElev, ok := p.GetMinElevation(); ok {
		printMinElevation(f, minElev)
	}
	if timePulse, ok := p.GetTimePulse(); ok {
		printTimePulse(f, timePulse)
	}
	if mode, ok := p.GetMode(); ok {
		printMode(f, mode)
	}
	if rtcmBaseID, ok := p.GetRTCMBaseID(); ok {
		printRTCMBaseID(f, rtcmBaseID)
	}
	if port, ok := p.GetPort(); ok {
		printPort(f, port)
	}
	if baudRate, ok := p.GetBaudRate(); ok {
		printBaudRate(f, baudRate)
	}
}

func printSignals(f *os.File, sigs gpsprot.SignalSet) {
	m := sigs.GNSSSignalMap()
	if len(m) == 0 {
		return
	}
	var constellations []string
	for g := gpsprot.GNSS(1); g <= gpsprot.GNSSLast; g++ {
		if _, ok := m[g.String()]; ok {
			constellations = append(constellations, g.String())
		}
	}
	fmt.Fprintf(f, "Constellations enabled: %s\n", strings.Join(constellations, ", "))
	for g := gpsprot.GNSS(1); g <= gpsprot.GNSSLast; g++ {
		if sigNames, ok := m[g.String()]; ok {
			fmt.Fprintf(f, "%s signals enabled: %s\n", g.String(), strings.Join(sigNames, ", "))
		}
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
	case gpsprot.PosTypeLLH:
		fmt.Fprintf(f, "Fixed position LLH: %s,%s,%s\n", mode.FixedPosLLH[0].String(), mode.FixedPosLLH[1].String(), mode.Height.String())
	}
	fmt.Fprintf(f, "Fixed position accuracy: %s m\n", mode.FixedPosAcc.String())
}

func printTimeGNSS(f *os.File, timeGNSS gpsprot.GNSS) {
	fmt.Fprintf(f, "Time GNSS: %s\n", timeGNSS.String())
}

func printAntennaCableDelay(f *os.File, delay time.Duration) {
	fmt.Fprintf(f, "Antenna cable delay: %d ns\n", delay.Nanoseconds())
}

func printMinElevation(f *os.File, elev gpsprot.Angle) {
	fmt.Fprintf(f, "Minimum elevation: %s\n", elev.String())
}

func printRTCMBaseID(f *os.File, id uint16) {
	fmt.Fprintf(f, "RTCM base station ID: %d\n", id)
}

func printPort(f *os.File, name string) {
	fmt.Fprintf(f, "Port: %s\n", name)
}

func printBaudRate(f *os.File, baudRate uint32) {
	if baudRate == 0 {
		return
	}
	fmt.Fprintf(f, "Serial speed: %d\n", baudRate)
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

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, conn gpsio.Conn, pLog *gpsio.PacketLog, pktFormats []gpsprot.PacketFormat) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	wg.Go(func() { gpsio.Scan(ctx, lg, conn, msg, pLog, pktFormats) })
	return msg
}

func keepReading(ctx context.Context, lg *slog.Logger, pCh <-chan scan.Packet, dur time.Duration, rh *responseHandler) {
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
			if rh != nil {
				rh.handlePacket(pkt)
			}
		}
	}
}
