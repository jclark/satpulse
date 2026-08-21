package gpscmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/gpscfg"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
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
	vendors, err := cmd.ResolveVendors(v.vendor)
	if err != nil {
		return
	}
	var target *gpsprot.ConfigTarget
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
	} else {
		target = createConfigTarget(v)
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
	err = run(ctx, lg, target, raw, conn, vendors, v.packetLogPath, v.packetLogMode, v.capture, v.showReceiver, v.jsonOut, v.configSupport, args)
	return
}

func createConfigTarget(v *flagVars) *gpsprot.ConfigTarget {
	target := v.targetJSON
	if target != nil {
		target.Get |= v.configGet
	} else {
		target = gpsprot.NewConfigTarget()
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
	}
	// Opts.Socket describes the transport, not a configuration request, so
	// it is set from the transport even for a JSON target.
	target.Opts.Socket = v.socketPath != ""
	if target.NoOp() {
		target.Opts.ForceProbe = true
	}
	return target
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
// Modes based on raw:
//   - raw non-nil: message file mode (sends user-defined messages)
//   - raw nil: config mode (runs GPS detection/configuration)
//
// Parameter dependencies:
//   - logMode: must not be testLogMode when raw is non-nil
//   - args: only used for test log header when logMode is testLogMode
//   - target: non-nil when raw is nil
func run(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, raw []msgfile.RawMsg, conn gpsio.Conn, vendors []gpsreg.Vendor, logPath string, logMode packetLogMode, capture opt.Val[time.Duration], showReceiver bool, jsonOut bool, support configSupportReq, args []string) error {
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

	pktFormats := gpsreg.CreatePacketFormats(vendors)
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
	} else {
		rslt, err = runConfig(ctx, lg, target, pCh, conn, vendors, capture, showReceiver, jsonOut, support)
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

func runConfig(ctx context.Context, lg *slog.Logger, target *gpsprot.ConfigTarget, pCh <-chan scan.Packet, conn gpsio.Conn, vendors []gpsreg.Vendor, capture opt.Val[time.Duration], showReceiver bool, jsonOut bool, support configSupportReq) (*gpscfg.Result, error) {
	// Compile-time check: serial faults surfaced by gpsio satisfy the
	// gpscfg.SerialError interface. gpscfg relies on this.
	var _ gpscfg.SerialError = (*gpsio.SerialError)(nil)
	pktProcs := gpsreg.CreatePacketProcessors(vendors)
	gpsprot.SetAllMsgHandlers(pktProcs, &gpsprot.DefaultHandler{})
	rslt, err := gpscfg.Configure(ctx, lg, pktProcs, gpsreg.CreateConfigProtocols(vendors), target, pCh, conn)
	if errors.Is(err, gpscfg.ErrNoProbeResponse) && configTargetIsProbeOnly(target) {
		err = nil
	}
	var warnings []string
	if err == nil && rslt != nil {
		warnMissingConfigSupport(lg, support, rslt.ConfigSupport)
		if !configTargetIsProbeOnly(target) {
			warnings = configWarnings(target, rslt)
			for _, w := range warnings {
				lg.Warn(w)
			}
		}
	}
	if jsonOut {
		printJSON(os.Stdout, rslt, err, warnings)
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

// configWarnings derives user-facing warnings by comparing what the
// target asked for against the receiver's declared support and its
// post-configuration state. Only real problems warn: a request the
// receiver already satisfies produces nothing. On onlyWithReset
// receivers (see the gpsprot qualifier flags) a stored-only setting
// requested without --save plus --reload/--reset is skipped by the
// Configurator and warns with the missing flags: as a
// request-vs-state difference where the skip is detectable, and
// unconditionally for signals, where it is not.
func configWarnings(target *gpsprot.ConfigTarget, rslt *gpscfg.Result) []string {
	props := rslt.ConfigProps
	if props == nil {
		return nil
	}
	var ws []string
	opts := &target.Opts
	saveReset := opts.Save != gpsprot.SaveNone && (opts.Reset == gpsprot.ResetReload || opts.Reset == gpsprot.ResetCold)
	onlyWithReset := func(what string) {
		ws = append(ws, what+" was not performed: on this receiver it takes effect only after a save and reset (add --save with --reload or --reset)")
	}
	if reqSigs, ok := target.Props.GetSignalsEnabled(); ok {
		// A skipped signal change cannot be detected from the state:
		// the request over-specifies (--gnss expands to every band)
		// and the receiver's supported signals are unknown, so whether
		// the as-found state already satisfies the request is
		// undecidable. Warn pessimistically.
		if rslt.ConfigSupport&gpsprot.ConfigSupportSignalOnlyWithReset != 0 && !saveReset {
			ws = append(ws, "signals cannot be changed on this receiver without a save and reset (add --save with --reload or --reset)")
		} else if rsltSigs, ok := props.GetSignalsEnabled(); ok && reqSigs.GNSSSet() != rsltSigs.GNSSSet() {
			ws = append(ws, "only some of the requested constellations were enabled; the receiver does not support enabling all of them")
		}
	}
	if rslt.ConfigSupport&gpsprot.ConfigSupportModeOnlyWithReset != 0 && !saveReset {
		if m, ok := props.GetMode(); ok && modeUnmet(target, m) {
			onlyWithReset("the positioning mode change")
		}
	}
	if opts.RTCMMsg.IsSet() {
		flags := opts.RTCMMsg.Get()
		want := 0
		if flags&gpsprot.RTCMMsgMSM7 != 0 {
			want = 7
		} else if flags&gpsprot.RTCMMsgMSM4 != 0 {
			want = 4
		}
		// An explicit MSM type is a demand; RTCMMsgLax is a preference
		// that degrades silently. The effective type is a boot-time
		// constant, so any MSM message observed during configuration
		// reveals it; with none observed, warn pessimistically.
		if want != 0 && flags&gpsprot.RTCMMsgLax == 0 &&
			rslt.ConfigSupport&gpsprot.ConfigSupportRTCMMSMOnlyWithReset != 0 && !saveReset &&
			observedMSMLevel(rslt.ReceiverInfo) != want {
			onlyWithReset(fmt.Sprintf("the MSM%d selection", want))
		}
	}
	return ws
}

// modeUnmet reports whether the receiver's effective mode differs
// from the requested positioning mode in kind (mobile, survey, or
// fixed position). Positions are not compared: the receiver stores
// them quantized, and LLH requests may be realized as ECEF.
func modeUnmet(target *gpsprot.ConfigTarget, m gpsprot.Mode) bool {
	if req, ok := target.Props.GetMode(); ok {
		return modeKind(req) != modeKind(m)
	}
	// SetStatic asks for static mode without disturbing an existing
	// fixed position, so any static mode satisfies it.
	return target.Opts.SetStatic && !m.Static
}

func modeKind(m gpsprot.Mode) int {
	if !m.Static {
		return 0
	}
	if m.PosType == gpsprot.PosTypeNone {
		return 1 // survey
	}
	return 2 // fixed position
}

// observedMSMLevel returns the MSM level of the RTCM messages
// received during configuration, or 0 if none were seen.
func observedMSMLevel(info *gpsprot.ReceiverInfo) int {
	if info == nil {
		return 0
	}
	for _, id := range info.MsgTypes[gpsreg.TagRTCM] {
		if n, err := strconv.Atoi(id); err == nil {
			if lvl := rtcmbin.MsgType(n).MSMLevel(); lvl != 0 {
				return lvl
			}
		}
	}
	return 0
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
