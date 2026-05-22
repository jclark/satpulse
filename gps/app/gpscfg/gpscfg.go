package gpscfg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/internal/nmea"
	"github.com/jclark/satpulse/gps/scan"
	"golang.org/x/exp/maps"
)

type Result struct {
	ReceiverInfo          *gpsprot.ReceiverInfo
	ConfigSupport         gpsprot.ConfigSupportFlags
	ConfigProps           *gpsprot.ConfigProps
	PacketFormatsDetected []gpsprot.Tag
}

type msgHandler struct {
	lg          *slog.Logger
	packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor
	configProts []gpsprot.ConfigProtocol
	packetCh    <-chan scan.Packet
	msgCount    map[gpsprot.Tag]int
	bad         badCount
	msgIDs      map[gpsprot.Tag]map[string]bool
}

type badCount struct {
	invalidBytes, corruptMsgs, framingErrs int
}

var _ gpsprot.NativeMsgHandler = &msgHandler{}

var ErrNoProbeResponse = errors.New("no response to configuration probe message; not configuring GPS")
var ErrNotDetected = errors.New("GPS detection failed")

func Configure(ctx context.Context, lg *slog.Logger, packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor, configProts []gpsprot.ConfigProtocol, target *gpsprot.ConfigTarget, packetCh <-chan scan.Packet, port gpsio.OutPort) (*Result, error) {
	if ro := target.Props.ReadOnlyProps(); ro != 0 {
		panic(fmt.Sprintf("read-only properties in target.Props: %v", ro))
	}
	mh := msgHandler{}
	mh.init(lg, packetProcs, configProts, packetCh)

	noop := target.NoOp()
	probeEnabled := !noop && len(mh.configProts) > 0
	socket := target.Opts.Socket

	if probeEnabled && port.ReadOnly() {
		lg.Warn("device is not writable, not configuring GPS")
		probeEnabled = false
	}

	// Install native message handlers before detection if probing is enabled
	var mnmh *gpsprot.MultiNativeMsgHandler
	if probeEnabled {
		var restore func()
		mnmh, restore = mh.installNativeMsgHandlers()
		defer restore()
	}

	configProt, err := mh.detect(ctx, port, probeEnabled, socket)
	if err != nil {
		return nil, err
	}

	if !probeEnabled {
		return mh.noProbeResult(), nil
	}

	if configProt == nil {
		return mh.noProbeResult(), ErrNoProbeResponse
	}

	// During probing, MultiNativeMsgHandler fans out native messages to mh + all config protocols.
	// Now that we know which protocol succeeded, reset to just mh + that one protocol.
	mnmh.Reset(&mh, configProt)

	cfgProps, rcvrInfo, support, err := mh.configure(ctx, configProt, target, port)
	// We have no ConfigProps and no ReceiverInfo, so return a nil result.
	if err != nil && cfgProps == nil {
		return nil, err
	}
	// Return the configuration results, even if there were errors during configuration.
	return mh.finish(cfgProps, rcvrInfo, support), err
}

func (mh *msgHandler) init(lg *slog.Logger, packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor, configProts []gpsprot.ConfigProtocol, packetCh <-chan scan.Packet) {
	mh.lg = lg
	mh.packetProcs = packetProcs
	mh.packetCh = packetCh
	mh.configProts = configProts
	mh.msgCount = map[gpsprot.Tag]int{}
	mh.msgIDs = map[gpsprot.Tag]map[string]bool{}
	for tag := range packetProcs {
		mh.msgCount[tag] = 0
		mh.msgIDs[tag] = map[string]bool{}
	}
}

func (mh *msgHandler) finish(cfgProps *gpsprot.ConfigProps, rcvrInfo *gpsprot.ReceiverInfo, support gpsprot.ConfigSupportFlags) *Result {
	lg := mh.lg
	if cfgProps != nil {
		lg.Info("GPS configuration", "cfg", cfgProps)
	}
	if rcvrInfo != nil {
		lg.Info("GPS receiver", "vendor", rcvrInfo.Vendor, "hardware", rcvrInfo.Hardware,
			"firmware", rcvrInfo.Firmware, "gnss", rcvrInfo.SupportedGNSS, "configSupport", support)
	}
	for tag, msgIDs := range mh.msgIDs {
		if len(msgIDs) > 0 {
			lg.Info("message types received during configuration", "protocol", tag, "msgIDs", maps.Keys(msgIDs))
		}
	}
	return &Result{
		ReceiverInfo:          rcvrInfo,
		ConfigSupport:         support,
		ConfigProps:           cfgProps,
		PacketFormatsDetected: mh.packetFormatsDetected(),
	}
}

// noProbeResult returns a Result when there was no probe or the probe failed.
func (mh *msgHandler) noProbeResult() *Result {
	return &Result{
		ConfigProps:           new(gpsprot.ConfigProps),
		PacketFormatsDetected: mh.packetFormatsDetected(),
	}
}

func (mh *msgHandler) packetFormatsDetected() []gpsprot.Tag {
	tags := make([]gpsprot.Tag, 0, len(mh.msgCount))
	for tag, count := range mh.msgCount {
		if count > 0 {
			tags = append(tags, tag)
		}
	}
	slices.Sort(tags)
	return tags
}

// detect detects the GPS receiver, optionally sending probe packets.
// When probeEnabled is true, probe packets are sent during detection.
// When socket is true, the connection is via socket (daemon already validated it).
// Returns the ConfigProtocol that responded to probing (nil if probing disabled or timed out),
// and an error if detection failed.
func (mh *msgHandler) detect(ctx context.Context, port gpsio.OutPort, probeEnabled bool, socket bool) (gpsprot.ConfigProtocol, error) {
	if socket && !probeEnabled {
		return nil, nil
	}
	base := detector{
		mh:            mh,
		deadlineTimer: time.After(listenTimeout),
		maxDeadline:   time.Now().Add(listenMaxTimeout),
	}
	var configProt gpsprot.ConfigProtocol
	var bd *detector
	var err error
	if probeEnabled {
		pd := &probingDetector{
			detector:     base,
			port:         port,
			silenceTimer: time.After(silentWaitTimeout),
		}
		if socket {
			if err := pd.maybeSendProbe(0); err != nil {
				return nil, err
			}
			pd.validPacketReceived = true
			pd.silenceTimer = nil
			pd.deadlineTimer = nil
		}
		configProt, err = pd.run(ctx)
		bd = &pd.detector
	} else {
		ld := &listeningDetector{detector: base}
		err = ld.run(ctx)
		bd = &ld.detector
	}
	if err != nil {
		return nil, err
	}
	if socket {
		return configProt, nil
	}
	// Check ongoing errors (warn, not fatal)
	if bd.validPacketReceived {
		badNew := mh.bad.Sub(bd.badStart)
		if badNew.framingErrs > 0 {
			mh.lg.Warn("ongoing framing errors reading GPS output (hardware problems?)")
		}
		if badNew.corruptMsgs > 0 {
			mh.lg.Warn("ongoing corrupted GPS output (multiple processes reading from serial port?)")
		}
	}
	if configProt != nil {
		return configProt, nil
	}
	if mh.suitableMessageCount() == 0 {
		var msg string
		if mh.bad.framingErrs > 0 {
			msg = "framing errors reading GPS output (wrong speed?)"
		} else if nativeOnly := mh.nativeOnlyTags(); len(nativeOnly) > 0 {
			msg = fmt.Sprintf("only messages with these protocols detected: %s", strings.Join(nativeOnly, ", "))
		} else if mh.bad.invalidBytes+mh.bad.corruptMsgs == 0 {
			msg = "no output detected from GPS"
		} else if mh.bad.corruptMsgs > 0 {
			msg = "corrupted GPS output (multiple processes reading from serial port?)"
		} else {
			msg = "cannot parse GPS output"
		}
		mh.lg.Debug("not receiving data from GPS correctly", "bad", mh.bad, "nativeOnlyTags", mh.nativeOnlyTags())
		return nil, fmt.Errorf("%w: %s", ErrNotDetected, msg)
	}
	mh.lg.Info("detected a GPS")
	mh.lg.Debug("received suitable output message from GPS", "msgCount", mh.msgCount, "bad", mh.bad)
	return configProt, nil
}

func (mh *msgHandler) packetChClosed(ctx context.Context) error {
	mh.packetCh = nil
	err := ctx.Err()
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

const (
	// listenTimeout is the initial deadline for detection (both modes)
	listenTimeout = 2 * time.Second
	// listenMaxTimeout is the absolute cap for deadline extensions on framing errors
	listenMaxTimeout = 10 * time.Second
	// silentWaitTimeout is how long to wait with no input before sending the first probe
	silentWaitTimeout = 1 * time.Second
	// probeRetryDelay is how long to wait after sending a probe before retrying
	probeRetryDelay = 1500 * time.Millisecond
	// probeResponseTimeout is how long to wait after the final probe before giving up
	probeResponseTimeout = 3 * time.Second
)

// listeningDetector detects by listening for a suitable packet.
type listeningDetector struct {
	detector
}

// probingDetector detects by sending probe packets and listening for responses.
type probingDetector struct {
	detector
	port         gpsio.OutPort
	silenceTimer <-chan time.Time
	probeTimer   <-chan time.Time
	nProbesSent  int
}

// detector holds shared state for both detection modes.
type detector struct {
	mh                  *msgHandler
	deadlineTimer       <-chan time.Time
	maxDeadline         time.Time
	validPacketReceived bool
	badStart            badCount
	totalMsgCount       int
}

// run listens for packets until a suitable one arrives or the deadline fires.
func (d *listeningDetector) run(ctx context.Context) error {
	mh := d.mh
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case packet, ok := <-mh.packetCh:
			if !ok {
				return mh.packetChClosed(ctx)
			}
			d.processPacket(packet)
			if mh.suitableMessageCount() > 0 {
				return nil
			}
		case <-d.deadlineTimer:
			d.deadlineTimer = nil
			return nil
		}
	}
}

// run listens for packets while managing probe sending and timeouts.
func (d *probingDetector) run(ctx context.Context) (gpsprot.ConfigProtocol, error) {
	mh := d.mh
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case packet, ok := <-mh.packetCh:
			if !ok {
				return nil, mh.packetChClosed(ctx)
			}
			if err := d.processPacket(packet); err != nil {
				return nil, err
			}
			if prot := mh.probeSucceeded(); prot != nil {
				mh.lg.Debug("probe succeeded")
				return prot, nil
			}
		case <-d.silenceTimer:
			d.silenceTimer = nil
			mh.lg.Debug("silence timer fired")
			if err := d.maybeSendProbe(0); err != nil {
				return nil, err
			}
		case <-d.deadlineTimer:
			d.deadlineTimer = nil
			mh.lg.Debug("deadline timer fired")
			if err := d.maybeSendProbe(0); err != nil {
				return nil, err
			}
		case <-d.probeTimer:
			d.probeTimer = nil
			if d.nProbesSent >= 2 {
				mh.lg.Debug("no response to probes, giving up")
				return nil, nil
			}
			if err := d.maybeSendProbe(d.nProbesSent); err != nil {
				return nil, err
			}
		}
	}
}

// processPacket extends the base to cancel the silence timer on input
// and trigger the first probe when a valid packet arrives.
func (d *probingDetector) processPacket(packet scan.Packet) error {
	d.detector.processPacket(packet)
	if d.silenceTimer != nil && (len(packet.Data) > 0 || isFramingError(packet.ReadError)) {
		d.mh.lg.Debug("first input received, cancelling silence timer")
		d.silenceTimer = nil
	}
	if d.validPacketReceived {
		if err := d.maybeSendProbe(0); err != nil {
			return err
		}
	}
	return nil
}

// maybeSendProbe sends probes if nProbesSent equals probeIndex.
// It increments nProbesSent, nils deadline, and sets probeTimer for the next step.
func (d *probingDetector) maybeSendProbe(probeIndex int) error {
	if d.nProbesSent != probeIndex {
		return nil
	}
	for _, prot := range d.mh.configProts {
		if _, err := d.port.Write(prot.ProbePacket()); err != nil {
			return err
		}
	}
	d.nProbesSent++
	d.mh.lg.Debug("sent probe packets", "probeNum", d.nProbesSent)
	d.deadlineTimer = nil
	if d.nProbesSent == 1 {
		d.probeTimer = time.After(probeRetryDelay)
	} else {
		d.probeTimer = time.After(probeResponseTimeout)
	}
	return nil
}

// processPacket processes a packet, tracks valid packet receipt,
// and extends the deadline on framing errors.
func (d *detector) processPacket(packet scan.Packet) {
	mh := d.mh
	countBefore := d.totalMsgCount
	mh.packet(packet)
	d.totalMsgCount = mh.totalMsgCount()
	if d.totalMsgCount > countBefore && !d.validPacketReceived {
		d.validPacketReceived = true
		d.badStart = mh.bad
		mh.lg.Debug("first valid packet received", "tag", packet.Format.Tag())
	}
	if isFramingError(packet.ReadError) && d.deadlineTimer != nil {
		remaining := time.Until(d.maxDeadline)
		if remaining > 0 {
			mh.lg.Debug("extending detection deadline due to framing errors")
			d.deadlineTimer = time.After(min(listenTimeout, remaining))
		}
	}
}

// isFramingError reports whether err is a serial error with framing errors.
func isFramingError(err error) bool {
	se, ok := err.(SerialError)
	return ok && se.SerialFraming()
}

// probeSucceeded returns the first ConfigProtocol that has responded to probing, or nil.
func (mh *msgHandler) probeSucceeded() gpsprot.ConfigProtocol {
	for _, prot := range mh.configProts {
		if prot.ProbeOK() {
			return prot
		}
	}
	return nil
}

func (mh *msgHandler) totalMsgCount() int {
	count := 0
	for _, c := range mh.msgCount {
		count += c
	}
	return count
}

// installNativeMsgHandlers creates a NativeMsgHandler than fans out to all config protocols.
// It returns the MultiNativeMsgHandler and a function to restore the previous state.
func (mh *msgHandler) installNativeMsgHandlers() (*gpsprot.MultiNativeMsgHandler, func()) {
	handlers := make([]gpsprot.NativeMsgHandler, len(mh.configProts)+1)
	handlers[0] = mh
	for i, prot := range mh.configProts {
		handlers[i+1] = prot
	}
	nmhSaved := make(map[gpsprot.Tag]gpsprot.NativeMsgHandler, len(mh.packetProcs))

	mnmh := gpsprot.NewMultiNativeMsgHandler(handlers...)
	for tag, pp := range mh.packetProcs {
		nmhSaved[tag] = pp.GetNativeMsgHandler()
		pp.SetNativeMsgHandler(mnmh)
	}

	return mnmh, func() {
		for tag, pp := range mh.packetProcs {
			pp.SetNativeMsgHandler(nmhSaved[tag])
		}
	}
}

// maximum number of times to retry a request that doesn't get a response
const maxTries = 3

func (mh *msgHandler) configure(ctx context.Context, prot gpsprot.ConfigProtocol, target *gpsprot.ConfigTarget, port gpsio.OutPort) (*gpsprot.ConfigProps, *gpsprot.ReceiverInfo, gpsprot.ConfigSupportFlags, error) {
	cfgtor, err := prot.Configure(target)
	if err != nil {
		return nil, nil, 0, err
	}
	director := gpsprot.NewConfigDirector(cfgtor, maxTries)
	var knownErr error // error that we know how to handle
	for action := range director.Actions() {
		director.AdvanceTimeTo(time.Now())
		switch action.Type {
		case gpsprot.ConfigActionSendRequest:
			var err error
			if serPort, ok := port.(*gpsio.SerialConn); ok && action.Speed != 0 {
				_, err = serPort.WriteThenChangeSpeed(action.Packet, action.Speed)
			} else {
				_, err = port.Write(action.Packet)
			}
			if err != nil {
				return nil, nil, 0, fmt.Errorf("failed to send configuration packet: %w", err)
			}
			cfgtor.Request(action.Index).SetSentTime(time.Now())
			mh.lg.Debug("sent configuration message", "index", action.Index, "len", len(action.Packet))
		case gpsprot.ConfigActionWaitUntil:
			timerCh := time.After(time.Until(action.Deadline))
			select {
			case <-ctx.Done():
				return nil, nil, 0, ctx.Err()
			case <-timerCh:
				// Continue to next iteration to check for state changes
			case packet, ok := <-mh.packetCh:
				if !ok {
					return nil, nil, 0, mh.packetChClosed(ctx)
				}
				mh.packet(packet)
				if packet.ChecksumValid {
					director.ValidPacketReceived(packet.TRead)
				}
			}

		case gpsprot.ConfigActionError:
			if knownErr == nil {
				mh.lg.Warn("GPS configuration failed", "err", action.Error)
				knownErr = action.Error
			}
		}
	}
	return cfgtor.ConfigProps(), cfgtor.ReceiverInfo(), cfgtor.ConfigSupport(), knownErr
}

func (mh *msgHandler) suitableMessageCount() int {
	count := 0
	for tag, msgCount := range mh.msgCount {
		if pp, ok := mh.packetProcs[tag]; ok && !pp.NativeOnly() {
			count += msgCount
		}
	}
	return count
}

func (mh *msgHandler) nativeOnlyTags() []string {
	var tags []string
	for tag, msgCount := range mh.msgCount {
		if msgCount > 0 {
			if pp, ok := mh.packetProcs[tag]; ok && pp.NativeOnly() {
				tags = append(tags, string(tag))
			}
		}
	}
	return tags
}

func (mh *msgHandler) packet(pkt scan.Packet) {
	if pkt.IsInterPacketTimeout() {
		mh.lg.Debug("inter-packet timeout detected during GPS configuration")
		for _, pp := range mh.packetProcs {
			pp.Idle(pkt.TRead)
		}
		return
	}
	data := pkt.Data
	if pkt.Format == nil {
		mh.invalid(data, pkt.ReadError)
		return
	}
	tag := pkt.Format.Tag()
	pp, ok := mh.packetProcs[tag]
	if !ok {
		mh.lg.Error("no processor registered for protocol", "protocol", tag)
		return
	}
	err := pkt.ChecksumError()
	if err != nil && !pkt.AltChecksumValid() {
		mh.lg.Warn(err.Error(), "tag", tag, "len", len(pkt.Data))
		mh.bad.corruptMsgs++
		return
	}
	// only count messages with good checksum
	mh.msgCount[tag]++
	msgID, err := pp.ProcessPacket(data, pkt.TRead)
	if err != nil {
		mh.lg.Warn("GPS packet cannot be parsed", gpsio.PacketWarnAttrs(err, pkt, msgID)...)
		return
	}

	mh.msgIDs[tag][msgID] = true
}

func (mh *msgHandler) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) error {
	if tag == gpsreg.TagNMEA {
		nmeaLog(mh.lg, msg.(*nmea.Sentence))
	}
	return nil
}

// Some number of unparseable bytes is normal.
// Log if we get more than this
const invalidBytesMaxExpected = 100

type SerialError interface {
	error
	SerialFraming() bool
}

func (mh *msgHandler) invalid(data string, readErr error) {
	bad := &mh.bad
	n := bad.invalidBytes
	bad.invalidBytes += len(data)
	if bad.invalidBytes > invalidBytesMaxExpected && n <= invalidBytesMaxExpected {
		mh.lg.Debug("unexpectedly large number of unparseable bytes while starting to read GPS output")
	}
	if readErr != nil {
		if err, ok := readErr.(SerialError); ok && err.SerialFraming() {
			if bad.framingErrs == 0 {
				mh.lg.Info("framing errors reading GPS output during initialization")
			}
			bad.framingErrs++
		} else {
			// Don't expect these
			mh.lg.Info("error reading GPS output during initialization", "err", readErr)
		}
	}
}

func (bc1 badCount) Sub(bc2 badCount) badCount {
	return badCount{
		invalidBytes: bc1.invalidBytes - bc2.invalidBytes,
		corruptMsgs:  bc1.corruptMsgs - bc2.corruptMsgs,
		framingErrs:  bc1.framingErrs - bc2.framingErrs,
	}
}

func nmeaLog(lg *slog.Logger, sen *nmea.Sentence) {
	approvSen := sen.ApprovedSentence()
	if approvSen != nil && approvSen.Format == "TXT" && len(approvSen.Fields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Debug("received NMEA TXT message", "s", approvSen.Fields[3])
	}
}
