package gpsprot

import "time"

// Tag identifies the kind of packet
type Tag string

const InvalidTag Tag = "Invalid"

// ScanState represents the state of a packet scanner
type ScanState uint32

// Constants for scan states
const (
	ScanStateSync ScanState = 0 // Always 0, the initial state looking for packet start
)

// PacketFormat defines the interface for protocol-specific packet format handling
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
}

// PacketProcessor processes packets of a specific protocol
type PacketProcessor interface {
	// ProcessPacket processes a packet's data and returns any error
	ProcessPacket(data string, tRead time.Time) error

	// SetMsgHandler sets the handler for protocol-agnostic messages
	SetMsgHandler(handler MsgHandler)

	// SetNativeMsgHandler sets the handler for protocol-specific messages
	SetNativeMsgHandler(handler NativeMsgHandler)

	// GetNativeMsgHandler returns the current protocol message handler
	GetNativeMsgHandler() NativeMsgHandler

	// CreatePacketExchanger creates a PacketExchanger if configuration is supported
	// Returns nil if configuration is not supported
	CreatePacketExchanger() PacketExchanger
}

// DefaultPacketProcessor provides default implementations of PacketProcessor methods
type DefaultPacketProcessor struct {
	nmh NativeMsgHandler
}

// ProcessPacket provides a default implementation that does nothing and returns nil
func (p *DefaultPacketProcessor) ProcessPacket(data string, tRead time.Time) error {
	return nil
}

// SetMsgHandler does nothing in the default implementation
func (p *DefaultPacketProcessor) SetMsgHandler(handler MsgHandler) {
	// Intentionally empty
}

// SetNativeMsgHandler sets the native message handler
func (p *DefaultPacketProcessor) SetNativeMsgHandler(handler NativeMsgHandler) {
	p.nmh = handler
}

// GetNativeMsgHandler returns the current native message handler
func (p *DefaultPacketProcessor) GetNativeMsgHandler() NativeMsgHandler {
	return p.nmh
}

// CreatePacketExchanger returns nil by default as most protocols don't support configuration
func (p *DefaultPacketProcessor) CreatePacketExchanger() PacketExchanger {
	return nil
}
