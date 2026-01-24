package gpscfg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/nmea"
	"github.com/jclark/satpulse/internal/scan"
	"golang.org/x/exp/maps"
)

type Result struct {
	ReceiverInfo          *gpsprot.ReceiverInfo
	ConfigProps           *gpsprot.ConfigProps
	LeapSecond            *gpsprot.LeapSecondMsg
	PacketFormatsDetected []gpsprot.Tag
}

type msgHandler struct {
	gpsprot.DefaultHandler
	lg          *slog.Logger
	packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor
	configProts []gpsprot.ConfigProtocol
	packetCh    <-chan scan.Packet
	msgCount    map[gpsprot.Tag]int
	bad         badCount
	msgIDs      map[gpsprot.Tag]map[string]bool
	leapSecond  *gpsprot.LeapSecondMsg
}

type badCount struct {
	invalidBytes, corruptMsgs, framingErrs int
}

var _ gpsprot.NativeMsgHandler = &msgHandler{}

var ErrNoProbeResponse = errors.New("no response to configuration probe message; not configuring GPS")
var ErrNotDetected = errors.New("GPS detection failed")

func Configure(ctx context.Context, lg *slog.Logger, packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor, configProts []gpsprot.ConfigProtocol, target *gpsprot.ConfigTarget, packetCh <-chan scan.Packet, port gpsio.OutPort) (*Result, error) {
	mh := msgHandler{}
	mh.init(lg, packetProcs, configProts, packetCh)
	var err error
	if !target.Opts.Detected {
		err = mh.detect(ctx)
	}
	noop := target.NoOp()
	if err != nil {
		// even with NoOp, we want to run detect in order to deal with framing errors
		if noop {
			// there's no point in giving up with NoOp, maybe we can bring it back to life using satpulsetool
			lg.Warn(err.Error())
		} else if target.Opts.ForceProbe&gpsprot.ForceProbeWhenNoOutput != 0 {
			lg.Info("no output detected from GPS, but trying to probe GPS anyway")
		} else {
			return nil, err
		}
	}
	if noop {
		// no need to probe
		return mh.noProbeResult(), nil
	}
	// After we have done detection, bad stuff is a cause for concern.
	badStart := mh.bad
	// Set up a MultiNativeMsgHandler to fan out to all protocols during probing
	mnmh, restore := mh.installNativeMsgHandlers()
	defer restore()
	configProt, err := mh.probe(ctx, port)
	if err != nil {
		return nil, err
	}
	badNew := mh.bad.Sub(badStart)
	if badNew.framingErrs > 0 {
		return nil, errors.New("ongoing framing errors reading GPS output (hardware problems?)")
	}
	if badNew.corruptMsgs > 0 {
		return nil, errors.New("ongoing corrupted GPS output (multiple processes reading from serial port?)")
	}
	if configProt == nil {
		// XXX if msgCount for UBX > 0, but probe failed, then probably we cannot send to the GPS
		return mh.noProbeResult(), ErrNoProbeResponse
	}
	mnmh.Reset(&mh, configProt)
	cfgProps, rcvrInfo, err := mh.configure(ctx, configProt, target, port)
	// We have no ConfigProps and no ReceiverInfo, so return a nil result.
	if err != nil && cfgProps == nil {
		return nil, err
	}
	// Return the configuration results, even if there were errors during configuration.
	return mh.finish(cfgProps, rcvrInfo), err
}

func (mh *msgHandler) init(lg *slog.Logger, packetProcs map[gpsprot.Tag]gpsprot.PacketProcessor, configProts []gpsprot.ConfigProtocol, packetCh <-chan scan.Packet) {
	mh.lg = lg
	mh.packetProcs = packetProcs
	mh.packetCh = packetCh
	mh.configProts = configProts
	mh.msgCount = map[gpsprot.Tag]int{}
	mh.msgIDs = map[gpsprot.Tag]map[string]bool{}
	for tag, pp := range packetProcs {
		pp.SetMsgHandler(mh)
		mh.msgCount[tag] = 0
		mh.msgIDs[tag] = map[string]bool{}
	}
}

func (mh *msgHandler) finish(cfgProps *gpsprot.ConfigProps, rcvrInfo *gpsprot.ReceiverInfo) *Result {
	lg := mh.lg
	if cfgProps != nil {
		lg.Info("GPS configuration", "cfg", cfgProps)
	}
	if mh.leapSecond != nil {
		lsdStr := mh.leapSecond.Date().Format("2006-01-02")
		lg.Info("leap second information received from GPS", "date", lsdStr, "utcOffBefore", mh.leapSecond.UTCOffBefore, "utcOffAfter", mh.leapSecond.UTCOffAfter)
	}
	if rcvrInfo != nil {
		lg.Info("GPS receiver", "vendor", rcvrInfo.Vendor, "hardware", rcvrInfo.Hardware,
			"firmware", rcvrInfo.Firmware, "gnss", rcvrInfo.SupportedGNSS)
	}
	for tag, msgIDs := range mh.msgIDs {
		if len(msgIDs) > 0 {
			lg.Info("message types received during configuration", "protocol", tag, "msgIDs", maps.Keys(msgIDs))
		}
	}
	return &Result{
		ReceiverInfo:          rcvrInfo,
		ConfigProps:           cfgProps,
		LeapSecond:            mh.leapSecond,
		PacketFormatsDetected: mh.packetFormatsDetected(),
	}
}

// noProbeResult returns a Result when there was no probe or the probe failed.
func (mh *msgHandler) noProbeResult() *Result {
	return &Result{
		ConfigProps:           new(gpsprot.ConfigProps),
		LeapSecond:            mh.leapSecond,
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

func (mh *msgHandler) detect(ctx context.Context) error {
	// Validate that we are receiving data correctly from a GPS.
	// The criteria for this is that we get a NMEA or UBX message with a valid checksum
	// (not necessarily a message that we understand).
	// This stage finishes as soon as we get such a message.
	// I have found that when starting to read from a GPS, there can be invalid bytes to start with:
	// one possible cause of this if that we start reading in the middle of the message;
	// I think there may also be UART issues that cause framing errors when starting to read.
	// We allow 2 seconds if there is no framing error.
	// We allow 15 seconds if we get a framing error.
	timer1Ch := time.After(time.Second * 2)
	timer2Ch := time.After(time.Second * 15)
	for mh.packetCh != nil {
		select {
		case packet, ok := <-mh.packetCh:
			if !ok {
				return mh.packetChClosed(ctx)
			}
			mh.packet(packet)
		case <-timer1Ch:
			timer1Ch = nil
		case <-timer2Ch:
			timer2Ch = nil
		}
		if mh.suitableMessageCount() > 0 || timer2Ch == nil || (timer1Ch == nil && mh.bad.framingErrs == 0) {
			break
		}
	}
	lg := mh.lg
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
		lg.Debug("not receiving data from GPS correctly", "bad", mh.bad, "nativeOnlyTags", mh.nativeOnlyTags())
		return fmt.Errorf("%w: %s", ErrNotDetected, msg)
	}
	lg.Info("detected a GPS")

	lg.Debug("received suitable output message from GPS", "msgCount", mh.msgCount, "bad", mh.bad)
	return nil
}

func (mh *msgHandler) packetChClosed(ctx context.Context) error {
	mh.packetCh = nil
	err := ctx.Err()
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

func (mh *msgHandler) probe(ctx context.Context, port gpsio.OutPort) (gpsprot.ConfigProtocol, error) {
	if len(mh.configProts) == 0 {
		return nil, nil
	}
	// Send probe packets for all protocols
	for _, prot := range mh.configProts {
		msg := prot.ProbePacket()
		_, err := port.Write(msg)
		if err != nil {
			return nil, err
		}
	}
	timerCh := time.After(time.Millisecond * 1500)
	for timerCh != nil {
		select {
		case packet, ok := <-mh.packetCh:
			if !ok {
				return nil, mh.packetChClosed(ctx)
			}
			mh.packet(packet)
			// Check if any protocol has succeeded
			for _, prot := range mh.configProts {
				if prot.ProbeOK() {
					return prot, nil
				}
			}
		case <-timerCh:
			timerCh = nil
		}
	}
	return nil, nil
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

func (mh *msgHandler) configure(ctx context.Context, prot gpsprot.ConfigProtocol, target *gpsprot.ConfigTarget, port gpsio.OutPort) (*gpsprot.ConfigProps, *gpsprot.ReceiverInfo, error) {
	cfgtor, err := prot.Configure(target)
	if err != nil {
		return nil, nil, err
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
				return nil, nil, fmt.Errorf("failed to send configuration packet: %w", err)
			}
			cfgtor.Request(action.Index).SetSentTime(time.Now())
			mh.lg.Debug("sent configuration message", "index", action.Index, "len", len(action.Packet))
		case gpsprot.ConfigActionWaitUntil:
			timerCh := time.After(time.Until(action.Deadline))
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-timerCh:
				// Continue to next iteration to check for state changes
			case packet, ok := <-mh.packetCh:
				if !ok {
					return nil, nil, mh.packetChClosed(ctx)
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
	return cfgtor.ConfigProps(), cfgtor.ReceiverInfo(), knownErr
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
	if err != nil && !altChecksumValid(pkt) {
		mh.lg.Warn(err.Error(), "tag", tag, "len", len(pkt.Data))
		mh.bad.corruptMsgs++
		return
	}
	// only count messages with good checksum
	mh.msgCount[tag]++
	msgID, err := pp.ProcessPacket(data, pkt.TRead)
	if err != nil {
		mh.lg.Error("GPS packet cannot be parsed", "protocol", tag, "err", err)
		return
	}

	mh.msgIDs[tag][msgID] = true
}

// altChecksumValid allows working around UM980 firmware bug which has incorrect checksum in some packets
func altChecksumValid(pkt scan.Packet) bool {
	if apf, ok := pkt.Format.(gpsprot.AltChecksumPacketFormat); ok {
		data := []byte(pkt.Data)
		if bytes.Equal(apf.ComputeAltChecksum(data), apf.ExtractChecksum(data)) {
			return true
		}
	}
	return false
}

func (mh *msgHandler) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) error {
	mh.lg.Debug("received an unused message during configuration stage", "protocol", tag, "msgID", msgID, "msg", msg)
	if tag == nmea.Tag {
		nmeaLog(mh.lg, msg.(*nmea.Sentence))
	}
	return nil
}

func (mh *msgHandler) LeapSecond(ls *gpsprot.LeapSecondMsg, _ time.Time) {
	mh.leapSecond = ls
}

// Some number of unparseable bytes is normal.
// Log if we get more than this
const invalidBytesMaxExpected = 100

type SerialError interface {
	error
	FramingErrs() int
}

func (mh *msgHandler) invalid(data string, readErr error) {
	bad := &mh.bad
	n := bad.invalidBytes
	bad.invalidBytes += len(data)
	if bad.invalidBytes > invalidBytesMaxExpected && n <= invalidBytesMaxExpected {
		mh.lg.Debug("unexpectedly large number of unparseable bytes while starting to read GPS output")
	}
	if readErr != nil {
		if err, ok := readErr.(SerialError); ok && err.FramingErrs() > 0 {
			if bad.framingErrs == 0 {
				mh.lg.Info("framing errors reading GPS output during initialization")
			}
			bad.framingErrs += err.FramingErrs()
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
