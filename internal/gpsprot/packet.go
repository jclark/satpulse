package gpsprot

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
