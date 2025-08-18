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
	if err != nil && !isKnownError(err) {
		return nil, err
	}
	// If we got a known error, then still return the configuration properties.
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
		return errors.New(msg)
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

func (mh *msgHandler) configure(ctx context.Context, prot gpsprot.ConfigProtocol, target *gpsprot.ConfigTarget, port gpsio.OutPort) (*gpsprot.ConfigProps, *gpsprot.ReceiverInfo, error) {
	cfgtor, err := prot.Configure(target)
	if err != nil {
		return nil, nil, err
	}
	var knownErr error // error that we know how to handle
	for {
		req, err := cfgtor.NextRequest()
		if err != nil {
			if knownErr == nil {
				mh.lg.Warn("GPS configuration failed", "err", err)
				knownErr = err
			}
			continue
		}
		if req == nil {
			break
		}
		err = mh.doRequest(ctx, cfgtor, req, port)
		if err != nil {
			if !isKnownError(err) {
				return nil, nil, err
			}
			if knownErr == nil {
				// the configurator doesn't need to be told about NACK; it gives up itself
				if !errors.Is(err, errNack) {
					mh.lg.Warn("GPS configuration failed", "msgID", req.ID(), "err", err)
					cfgtor.Abort()
				}
				knownErr = err
			}
			// keep going to allow the configurator to generate recovery requests
			continue
		}
	}
	return cfgtor.ConfigProps(), cfgtor.ReceiverInfo(), knownErr
}

const maxTries = 3

func (mh *msgHandler) doRequest(ctx context.Context, cfgtor gpsprot.Configurator, req gpsprot.ConfigRequest, port gpsio.OutPort) error {
	try := 0
	pkt := req.Packet()
	speed := req.ChangeSpeed()
	for {
		t := time.Now()
		var err error
		if serPort, ok := port.(*gpsio.SerialConn); ok && speed != 0 {
			_, err = serPort.WriteThenChangeSpeed(pkt, speed)
		} else {
			_, err = port.Write(pkt)
		}
		if err != nil {
			return err
		}
		err = mh.waitAfterSend(ctx, cfgtor, req, port, pkt, t)
		if err == nil {
			break
		}
		try++
		if try >= maxTries || speed != 0 { // don't retry speed change
			return err
		}
		if !errors.Is(err, errNoResponse) && !errors.Is(err, errNoAck) {
			return err
		}
		mh.lg.Info("retrying configuration request", "msgID", req.ID())
	}
	return nil
}

const minWaitAfterSend = 10 * time.Millisecond

var errNoResponse = errors.New("no response to configuration poll message")
var errNoAck = errors.New("no ack to configuration message")
var errNack = errors.New("configuration message rejected by GPS")

func isKnownError(err error) bool {
	return errors.Is(err, errNoResponse) || errors.Is(err, errNoAck) || errors.Is(err, errNack)
}

func (mh *msgHandler) waitAfterSend(ctx context.Context, cfgtor gpsprot.Configurator, req gpsprot.ConfigRequest, port gpsio.OutPort, pkt []byte, tSend time.Time) error {
	// tChecksumOK is the last time when we received a packet with valid checksum.
	// It is used for recovering from not receiving an ACK after a speed change.
	var tChecksumOK time.Time
	w := max(port.TransmitTime(len(pkt)), minWaitAfterSend)
	awaitingAck := req.Ackable()
	awaitingResp := req.AwaitingResponse(tSend)
	if awaitingAck || awaitingResp {
		w = time.Millisecond * 1500 // related to speedChangeDelay below
	}
	// speedChangeDelay is how long after tSend of a speed change packet till we assume a received packet
	// must have been sent by the receiver after the speed change.
	// Above, we wait for 1500ms for an ACK. If we don't get an ACK after a speed change,
	// then we look for valid checksum in packets received up to that time.
	// If speedChangeDelay is 400ms, then we have a window of 1100ms to receive a packet.
	// Since we assume the receiver will send at least one packet per second, this window
	// should be sufficient to receive a packet with valid checksum.
	const speedChangeDelay = 400 * time.Millisecond
	reqID := req.ID()
	mh.lg.Debug("sent configuration message", "msgID", reqID, "len", len(pkt))
	timerCh := time.After(w)
	var pauseCh <-chan time.Time
loop:
	for timerCh != nil {
		select {
		case packet, ok := <-mh.packetCh:
			if !ok {
				return mh.packetChClosed(ctx)
			}
			okChecksumCount := mh.totalMsgCount()
			mh.packet(packet)
			if mh.totalMsgCount() > okChecksumCount {
				tChecksumOK = packet.TRead
			}
			if awaitingResp {
				awaitingResp = req.AwaitingResponse(tSend)
			}
			if awaitingAck {
				ack := cfgtor.FindAck(pkt, tSend)
				if ack != nil {
					if !ack.OK {
						mh.lg.Warn("GPS configuration failed: message rejected", "msgID", reqID)
						return fmt.Errorf("%w: %s", errNack, reqID)
					}
					req.Done()
					mh.lg.Debug("configuration message accepted", "msgID", reqID, "delay", ack.TRead.Sub(tSend).String())
					awaitingAck = false
					pause := req.Pause()
					if pause > 0 {
						mh.lg.Debug("scheduling pause after configuration message", "msgID", reqID, "pause", pause)
						pauseCh = time.After(pause)
					}
					if awaitingResp {
						mh.lg.Info("configuration message acknowledged before poll response", "msgID", reqID)
					}
				}
			}
			if !awaitingAck && !awaitingResp {
				break loop
			}
		case <-timerCh:
			timerCh = nil
		}
	}
	if awaitingAck {
		if req.ChangeSpeed() == 0 {
			return fmt.Errorf("%w: %s", errNoAck, reqID)
		} else if tChecksumOK.Sub(tSend) < speedChangeDelay { // also handles tChecksumOK.IsZero()
			return fmt.Errorf("%w and valid packets not being received after speed change", errNoAck)
		}
		mh.lg.Debug("no ACK but packet with valid checksum received after speed change", "msgID", reqID)
	}
	if awaitingResp {
		return fmt.Errorf("%w: %s", errNoResponse, reqID)
	}
	if pauseCh != nil {
		<-pauseCh
		mh.lg.Debug("pause over", "msgID", reqID)
	}
	return nil
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

func (mh *msgHandler) totalMsgCount() int {
	count := 0
	for _, c := range mh.msgCount {
		count += c
	}
	return count
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
	if err != nil {
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
