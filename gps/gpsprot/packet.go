package gpsprot

import "time"

// Tag identifies the kind of packet
type Tag string

const EmptyTag Tag = ""

func (t Tag) IsZero() bool {
	return t == ""
}

// ScanState represents the state of a packet scanner
type ScanState uint32

// Constants for scan states
const (
	ScanStateSync ScanState = 0 // Always 0, the initial state looking for packet start
)

// PacketFormat defines the interface for protocol-specific packet format handling
// PacketFormat is immutable
type PacketFormat interface {
	// Tag returns the protocol-specific packet kind
	Tag() Tag

	// Next takes the current state, buffer, index, and packet length, and returns the next state
	// buf is the buffer containing the packet data
	// nextScanIndex is the index of the byte to process
	// packetLen is the length of the packet so far, not including the one at nextScanIndex
	Next(state ScanState, buf []byte, nextScanIndex, packetLen int) ScanState

	// IsFinal determines if the given state represents a complete packet
	IsFinal(state ScanState) bool

	// MsgID returns a human readable ID of the packet
	// Precondition: the packet must be valid according to Next()
	MsgID(pkt []byte) string

	// ExtractChecksum extracts the checksum from the packet
	// Precondition: the packet must be valid according to Next()
	ExtractChecksum(pkt []byte) []byte

	// ComputeChecksum computes the checksum for the given packet
	// Precondition: the packet must be valid according to Next()
	ComputeChecksum(pkt []byte) []byte

	// RescanOnBadChecksum determines if the scanner should rescan on a bad checksum
	// starting with the byte after the first byte of the packet with the bad checksum
	// prevPktValid indicates if the previous packet was valid
	RescanOnBadChecksum(prevPktValid bool, pkt []byte) bool
}

// MultiPacketFormat is implemented by packet formats where a single
// logical observation can be split across multiple packets.
// For RTCM MSM messages, the "multiple message" bit indicates that more
// packets follow for the same epoch.
type MultiPacketFormat interface {
	PacketFormat
	// HasMore returns true if the packet indicates that more
	// packets follow as part of the same logical group.
	HasMore(pkt []byte) bool
}

// AltChecksumPacketFormat provides an alternate method for computing the checksum,
// which can be used to work around firmware quirks.
// For examplem, on the Unicore UM980, the checksum for NMEA-like
// packets used for configuration responses computes the checksum over the packet
// including the leading $ (normal NMEA checksums exclude the $).
// ComputeAltChecksum in this case can detect when it might be one of these packets and
// return the checksum including the leading $.
type AltChecksumPacketFormat interface {
	PacketFormat
	// ComputeChecksum computes an alternate checksum for the given packet
	// Precondition: the packet must be valid according to Next()
	ComputeAltChecksum(pkt []byte) []byte
}

// IsValidPacket says whether the given buffer is a valid packet in the given PacketFormat.
// It uses only the Next method on PacketFormat.
// It does not validate the checksum.
func IsValidPacket(pf PacketFormat, buf []byte) bool {
	state := ScanStateSync
	for i := 0; i < len(buf); i++ {
		state = pf.Next(state, buf, i, i)
		if state == ScanStateSync {
			return false
		}
	}
	return pf.IsFinal(state)
}

// PacketProcessor processes packets of a specific protocol.
//
// Callers must call SetMsgHandler with a non-nil handler before
// calling ProcessPacket or Idle.
type PacketProcessor interface {
	// ProcessPacket processes a packet's data and returns a string with the type of the packet and an error
	ProcessPacket(data string, tRead time.Time) (string, error)

	// No data has been received for a period of time (usually 0.1s).
	// tRead is the end of the period.
	// There is no implication that data is received immediately after this.
	Idle(tRead time.Time)

	// SetMsgHandler sets the handler for protocol-agnostic messages
	SetMsgHandler(handler MsgHandler)

	// SetNativeMsgHandler sets the handler for protocol-specific messages
	SetNativeMsgHandler(handler NativeMsgHandler)

	// GetNativeMsgHandler returns the current protocol message handler
	GetNativeMsgHandler() NativeMsgHandler

	// NativeOnly returns true if this processor only generates native messages
	// and never produces protocol-agnostic messages like TimeMsg or SatellitesMsg.
	// This is typically true for protocols that provide correction data rather
	// than positioning solutions (e.g., RTCM).
	NativeOnly() bool
}

// DefaultPacketProcessor provides default implementations of PacketProcessor methods
type DefaultPacketProcessor struct {
	nmh NativeMsgHandler
}

// ProcessPacket provides a default implementation that does nothing and returns nil
func (p *DefaultPacketProcessor) ProcessPacket(data string, tRead time.Time) error {
	return nil
}

// Idle provides a default implementation that does nothing
func (p *DefaultPacketProcessor) Idle(tRead time.Time) {
	// Intentionally empty
}

// SetMsgHandler does nothing in the default implementation.
func (p *DefaultPacketProcessor) SetMsgHandler(handler MsgHandler) {
}

// SetNativeMsgHandler sets the native message handler
func (p *DefaultPacketProcessor) SetNativeMsgHandler(handler NativeMsgHandler) {
	p.nmh = handler
}

// GetNativeMsgHandler returns the current native message handler
func (p *DefaultPacketProcessor) GetNativeMsgHandler() NativeMsgHandler {
	return p.nmh
}

// NativeOnly returns false by default since most protocols provide timing/positioning
func (p *DefaultPacketProcessor) NativeOnly() bool {
	return false
}

// SetAllMsgHandlers sets handler on every PacketProcessor in the map.
func SetAllMsgHandlers(pps map[Tag]PacketProcessor, handler MsgHandler) {
	for _, pp := range pps {
		pp.SetMsgHandler(handler)
	}
}

type NMEAPacketProcessor interface {
	PacketProcessor
	// SetSVNumbering sets the satellite numbering scheme for NMEA.
	// The ranges must be non-overlapping and in increasing order of MinID/MaxID.
	SetSVNumbering([]NMEASVNumberingRange)
}

type NMEASVNumberingRange struct {
	MinID    uint16
	MaxID    uint16
	MinNum   uint8
	GNSS     GNSS
	SignalID SignalID
}
