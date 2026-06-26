// Package spartnbin parses the SPARTN (v2.0.3) transport frame envelope and
// computes its CRCs. SPARTN frames carry GNSS correction data, optionally
// encrypted (TF004/EAF). The whole envelope (TF001-TF015, TF017, TF018) is
// plaintext regardless of encryption; only the message payload (TF016) is
// encrypted. This package extracts the envelope metadata and returns the
// payload as opaque bytes; it does not decode or decrypt the payload.
//
// Frame layout (see SPARTN ICD section 7):
//
//	TF001 preamble (8)   TF002 type (7)      TF003 payload length (10)
//	TF004 EAF (1)        TF005 CRC type (2)  TF006 frame CRC-4 (4)
//	TF007 subtype (4)    TF008 time tag type (1)  TF009 time tag (16/32)
//	TF010 solution ID (7)  TF011 solution processor ID (4)
//	[when EAF=1] TF012 encryption ID (4)  TF013 encryption seq (6)
//	            TF014 auth indicator (3)  TF015 auth length (3)
//	TF016 payload (TF003 bytes)  [TF017 embedded auth]  TF018 CRC (8/16/24/32)
//
// The header through TF015 is always a whole number of bytes, so TF016 starts
// byte-aligned.
package spartnbin

import (
	"fmt"
	"iter"

	"github.com/jclark/satpulse/gps/lib/bitsenc"
)

// Preamble is the SPARTN frame preamble byte (TF001), ASCII 's'.
const Preamble = 0x73

const typeSubtypeSep = "."

// FrameStart holds TF001-TF006.
type FrameStart struct {
	Preamble   uint8
	Type       uint8  `bits:"7"`
	PayloadLen uint16 `bits:"10"`
	EAF        bool
	CRCType    uint8 `bits:"2"`
	FrameCRC   uint8 `bits:"4"`
}

// FrameMetadata holds TF007-TF011.
type FrameMetadata struct {
	Subtype        uint8  `bits:"4"`
	TimeTagType    uint8  `bits:"1"`
	TimeTag        uint32 `bits:"var"`
	SolutionID     uint8  `bits:"7"`
	SolutionProcID uint8  `bits:"4"`
}

// VarBits yields the bit width for TF009.
func (m *FrameMetadata) VarBits() iter.Seq[int] {
	return func(yield func(int) bool) { yield(timeTagBits(m.TimeTagType)) }
}

func timeTagBits(t uint8) int {
	if t == 1 {
		return 32
	}
	return 16
}

// FrameEncryption holds TF012-TF015.
type FrameEncryption struct {
	EncryptionID  uint8 `bits:"4"`
	EncryptionSeq uint8 `bits:"6"`
	AuthInd       uint8 `bits:"3"`
	AuthLen       uint8 `bits:"3"`
}

// Frame holds the parsed plaintext envelope of a SPARTN frame plus the opaque
// (possibly encrypted) message payload.
type Frame struct {
	FrameStart
	FrameMetadata
	FrameEncryption
	Payload []byte
}

// MsgID returns the SPARTN message identifier "type.subtype" (e.g. "1.0").
func (f *Frame) MsgID() string {
	return formatMsgID(f.Type, f.Subtype)
}

// Parse decodes the transport envelope of a complete SPARTN frame and slices
// out the message payload. The frame must be valid (preamble present and long
// enough); callers normally scan with the gpsprot.PacketFormat first.
func Parse(pkt []byte) (*Frame, error) {
	if len(pkt) < 5 || pkt[0] != Preamble {
		return nil, fmt.Errorf("spartnbin: not a SPARTN frame")
	}
	r := bitsenc.NewReader(pkt)
	f := new(Frame)
	if err := r.Read(&f.FrameStart); err != nil {
		return nil, err
	}
	if err := r.Read(&f.FrameMetadata); err != nil {
		return nil, err
	}
	if f.EAF {
		if err := r.Read(&f.FrameEncryption); err != nil {
			return nil, err
		}
	}
	hdrLen := byteLen(r.BitLen())
	payloadEnd := hdrLen + int(f.PayloadLen)
	if payloadEnd > len(pkt) {
		return nil, fmt.Errorf("spartnbin: payload exceeds frame")
	}
	f.Payload = pkt[hdrLen:payloadEnd]
	return f, nil
}

func byteLen(bits int) int { return (bits + 7) / 8 }

// FrameLen returns the total frame length in bytes, derived from the transport
// header at the start of hdr. ok is false when hdr is not a valid SPARTN frame
// prefix or does not yet contain enough header bytes to determine the payload,
// authentication, and CRC lengths. The scanner feeds successive header prefixes
// until ok.
func FrameLen(hdr []byte) (n int, ok bool) {
	if len(hdr) < 5 || hdr[0] != Preamble || !VerifyHeaderCRC(hdr) {
		return 0, false
	}
	var f Frame
	r := bitsenc.NewReader(hdr)
	if err := r.Read(&f.FrameStart); err != nil {
		return 0, false
	}
	if err := r.Read(&f.FrameMetadata); err != nil {
		return 0, false
	}
	if f.EAF {
		if err := r.Read(&f.FrameEncryption); err != nil {
			return 0, false
		}
	}
	return byteLen(r.BitLen()) + int(f.PayloadLen) + f.authLen() + crcBytes(f.CRCType), true
}

func (f *Frame) authLen() int {
	if f.EAF && f.AuthInd > 1 {
		return authBytes(uint16(f.AuthLen))
	}
	return 0
}

// authBytes returns the embedded authentication data length (TF017) in bytes
// for a TF015 value, or 0 for the reserved values 5-7.
func authBytes(tf015 uint16) int {
	switch tf015 {
	case 0:
		return 8 // 64 bits
	case 1:
		return 12 // 96 bits
	case 2:
		return 16 // 128 bits
	case 3:
		return 32 // 256 bits
	case 4:
		return 64 // 512 bits
	}
	return 0
}

// Type returns the message type (TF002). Precondition: len(pkt) >= 2.
func Type(pkt []byte) uint8 { return pkt[1] >> 1 }

// Subtype returns the message subtype (TF007). Precondition: len(pkt) >= 5.
func Subtype(pkt []byte) uint8 { return pkt[4] >> 4 }

// MsgID returns the message identifier "type.subtype" for a frame.
func MsgID(pkt []byte) string {
	return formatMsgID(Type(pkt), Subtype(pkt))
}

func formatMsgID(t, st uint8) string {
	return fmt.Sprintf("%d%s%d", t, typeSubtypeSep, st)
}

// ExtractChecksum returns the TF018 message CRC from the frame.
func ExtractChecksum(pkt []byte) []byte {
	return pkt[len(pkt)-crcLen(pkt):]
}

// ComputeChecksum computes the TF018 message CRC over TF002-TF017 (the frame
// without the preamble or trailing CRC), using the algorithm selected by TF005.
func ComputeChecksum(pkt []byte) []byte {
	n := crcLen(pkt)
	return checksum(pkt[3]>>4&3, pkt[1:len(pkt)-n])
}

// VerifyHeaderCRC checks the TF006 frame CRC-4 over TF002-TF005. hdr must hold
// at least the first 4 frame bytes.
func VerifyHeaderCRC(hdr []byte) bool {
	return headerCRC(hdr) == hdr[3]&0x0F
}

// Pack assembles a complete SPARTN frame from f, filling in the TF006 frame
// CRC and TF018 message CRC. It does not support embedded authentication data
// (TF017); f.AuthInd must be <= 1.
func Pack(f *Frame) ([]byte, error) {
	if len(f.Payload) > 1023 {
		return nil, fmt.Errorf("spartnbin: payload too long (%d bytes)", len(f.Payload))
	}
	if f.EAF && f.AuthInd > 1 {
		return nil, fmt.Errorf("spartnbin: embedded authentication not supported")
	}
	start := f.FrameStart
	start.Preamble = Preamble
	start.PayloadLen = uint16(len(f.Payload))
	start.FrameCRC = 0
	w := bitsenc.NewWriter(nil)
	if err := w.Write(&start); err != nil {
		return nil, err
	}
	if err := w.Write(&f.FrameMetadata); err != nil {
		return nil, err
	}
	if f.EAF {
		if err := w.Write(&f.FrameEncryption); err != nil {
			return nil, err
		}
	}
	pkt := w.Bytes()
	putHeaderCRC(pkt)
	pkt = append(pkt, f.Payload...)
	return append(pkt, checksum(start.CRCType, pkt[1:])...), nil
}

// crcLen returns the TF018 CRC length in bytes from TF005. Precondition:
// len(pkt) >= 4.
func crcLen(pkt []byte) int { return crcBytes(pkt[3] >> 4 & 3) }

func crcBytes(t uint8) int { return int(t) + 1 }

func putHeaderCRC(pkt []byte) {
	pkt[3] = pkt[3]&0xF0 | headerCRC(pkt)
}

func headerCRC(hdr []byte) uint8 {
	return crc4([]byte{hdr[1], hdr[2], hdr[3] & 0xF0})
}

// checksum computes the TF018 CRC of the given type over data and returns it
// in big-endian byte order.
func checksum(crcType uint8, data []byte) []byte {
	switch crcType {
	case 0: // CRC-8-CCITT
		return []byte{byte(crcMSB(data, 8, 0x07, 0, 0))}
	case 1: // CRC-16-CCITT
		v := crcMSB(data, 16, 0x1021, 0, 0)
		return []byte{byte(v >> 8), byte(v)}
	case 2: // CRC-24-Radix-64
		v := crcMSB(data, 24, 0x864CFB, 0, 0)
		return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
	default: // 3: CRC-32-CCITT
		v := crcMSB(data, 32, 0x04C11DB7, 0xFFFFFFFF, 0xFFFFFFFF)
		return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

// crcMSB computes an MSB-first (non-reflected) CRC of the given width over
// data. Used for the TF018 CRC-8/16/24/32 variants. width must be 8-32.
func crcMSB(data []byte, width int, poly, init, xorout uint64) uint64 {
	mask := uint64(1)<<width - 1
	top := uint64(1) << (width - 1)
	crc := init
	for _, b := range data {
		crc ^= uint64(b) << (width - 8)
		for range 8 {
			if crc&top != 0 {
				crc = crc<<1 ^ poly
			} else {
				crc <<= 1
			}
		}
		crc &= mask
	}
	return (crc ^ xorout) & mask
}

// crc4 computes the TF006 frame CRC-4 (polynomial 0x9, initial 0, reflected
// input and output) over data.
func crc4(data []byte) uint8 {
	crc := uint8(0)
	for _, b := range data {
		for i := range 8 {
			mix := (b>>uint(i) ^ crc) & 1
			crc >>= 1
			if mix != 0 {
				crc ^= 0x9
			}
		}
	}
	return crc & 0x0F
}
