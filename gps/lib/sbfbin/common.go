package sbfbin

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

const (
	Sync1 = 0x24
	Sync2 = 0x40
)

// Endian is the byte order used for SBF blocks.
var Endian = binary.LittleEndian

// MsgID is an SBF frame ID: 13-bit block number plus 3-bit revision.
type MsgID uint16

// Unpack returns the block number and revision encoded in mid.
func (mid MsgID) Unpack() (blockNumber uint16, rev uint8) {
	return uint16(mid & 0x1FFF), uint8(mid >> 13)
}

func (mid MsgID) String() string {
	n, _ := mid.Unpack()
	if s := idNameMap[MsgID(n)]; s != "" {
		return s
	}
	return fmt.Sprintf("%d", n)
}

// TimeStamp is the SBF Block Time Stamp.
type TimeStamp struct {
	TOW uint32
	WNc uint16
}

// Epoch returns the timestamp as a navigation epoch key.
func (ts TimeStamp) Epoch() (uint32, uint16) {
	return ts.TOW, ts.WNc
}

// TOWDNU and WNcDNU are the do-not-use sentinels of the block-header
// timestamp.
const (
	TOWDNU uint32 = 0xFFFFFFFF
	WNcDNU uint16 = 0xFFFF
)

// Params is a block's parameter set.
type Params interface {
	BlockNumber() uint16
}

// Block is a decoded SBF block.
type Block struct {
	Rev uint8 `json:"-"`
	TimeStamp
	Params Params
}

// ID returns the frame ID for b.
func (b *Block) ID() MsgID {
	return MsgID(b.Params.BlockNumber()) | MsgID(b.Rev)<<13
}

// Epoch returns the block timestamp as a navigation epoch key.
func (b *Block) Epoch() (uint32, uint16) {
	return b.TimeStamp.Epoch()
}

// UnknownParams holds an undecoded SBF block's parameter bytes.
type UnknownParams struct {
	Number  uint16
	Payload string
}

// BlockNumber returns the SBF block number.
func (p *UnknownParams) BlockNumber() uint16 {
	return p.Number
}

const (
	HeaderLen    = 8
	PacketMinLen = 16
)

var idNameMap = make(map[MsgID]string)
var blockMap = make(map[uint16]func() Params)

type defaulter interface {
	setDNUDefaults()
}

type payloadSizer interface {
	setPayloadLen(int)
	clearPayloadLen()
}

type payloadSize struct {
	n int
}

func (s *payloadSize) setPayloadLen(n int) {
	s.n = n
}

func (s *payloadSize) clearPayloadLen() {
	s.n = 0
}

func (s *payloadSize) payloadLen() int {
	return s.n
}

func regBlock[T any, PT interface {
	Params
	*T
}](idName string) {
	p := PT(new(T))
	n := p.BlockNumber()
	blockMap[n] = func() Params {
		p := PT(new(T))
		if d, ok := any(p).(defaulter); ok {
			d.setDNUDefaults()
		}
		return p
	}
	idNameMap[MsgID(n)] = idName
}

// Chunked is implemented by Params structs whose wire layout is not flat.
type Chunked interface {
	Chunks() func(yield func(chunk any) bool)
}

type chunkError string

func (e chunkError) Error() string { return string(e) }

func paddingChunk(n int) any {
	if n < 0 {
		return chunkError("sub-block length shorter than known fields")
	}
	if n == 0 {
		return nil
	}
	return make([]byte, n)
}

func setCount(n *uint8, count int, name string) any {
	if count == 0 {
		return nil
	}
	if count > 255 {
		return chunkError(fmt.Sprintf("%s count exceeds 255", name))
	}
	*n = uint8(count)
	return nil
}

func revisionChunks(payloadLen int, name string, fixed any, trailers ...any) func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		n := binary.Size(fixed)
		if payloadLen > 0 && payloadLen < n {
			yield(chunkError(fmt.Sprintf("payload shorter than %s fixed fields", name)))
			return
		}
		if !yield(fixed) {
			return
		}
		for _, chunk := range trailers {
			size := binary.Size(chunk)
			if payloadLen > 0 && payloadLen < n+size {
				return
			}
			if !yield(chunk) {
				return
			}
			n += size
		}
	}
}

func twoLevelChunks[Outer any, Inner any](
	n *uint8,
	sb1Len *uint8,
	sb2Len *uint8,
	head any,
	outer *[]Outer,
	inner *[][]Inner,
	getN2 func(*Outer) uint8,
	setN2 func(*Outer, uint8),
) func(yield func(chunk any) bool) {
	return func(yield func(chunk any) bool) {
		var outerZero Outer
		var innerZero Inner
		outerSize := binary.Size(outerZero)
		innerSize := binary.Size(innerZero)
		if err := setCount(n, len(*outer), "outer sub-block"); err != nil {
			yield(err)
			return
		}
		if *n > 0 && *sb1Len == 0 {
			*sb1Len = uint8(outerSize)
		}
		if *n > 0 && *sb2Len == 0 {
			*sb2Len = uint8(innerSize)
		}
		if !yield(head) {
			return
		}
		if len(*outer) != int(*n) {
			*outer = make([]Outer, int(*n))
		}
		if len(*inner) != int(*n) {
			*inner = make([][]Inner, int(*n))
		}
		sb1Pad := int(*sb1Len) - outerSize
		sb2Pad := int(*sb2Len) - innerSize
		for i := range *outer {
			o := &(*outer)[i]
			if len((*inner)[i]) > 255 {
				yield(chunkError("inner sub-block count exceeds 255"))
				return
			}
			if len((*inner)[i]) > 0 {
				setN2(o, uint8(len((*inner)[i])))
			}
			if !yield(o) {
				return
			}
			if !yield(paddingChunk(sb1Pad)) {
				return
			}
			if len((*inner)[i]) != int(getN2(o)) {
				(*inner)[i] = make([]Inner, int(getN2(o)))
			}
			for j := range (*inner)[i] {
				if !yield(&(*inner)[i][j]) {
					return
				}
				if !yield(paddingChunk(sb2Pad)) {
					return
				}
			}
		}
	}
}

func readBin(r *strings.Reader, v any) error {
	n := binary.Size(v)
	if n < 0 {
		return fmt.Errorf("variable-size binary chunk")
	}
	if n == 0 {
		return nil
	}
	if r.Len() < n {
		return io.ErrUnexpectedEOF
	}
	return binary.Read(r, Endian, v)
}

// ReadBinChunked reads binary data into msg.
func ReadBinChunked(r *strings.Reader, msg any, messageName string) error {
	if c, ok := msg.(Chunked); ok {
		for chunk := range c.Chunks() {
			if chunk == nil {
				continue
			}
			if err, ok := chunk.(chunkError); ok {
				return fmt.Errorf("parsing %s: %v", messageName, err)
			}
			err := readBin(r, chunk)
			if err != nil {
				return fmt.Errorf("parsing %s: %v", messageName, err)
			}
		}
		return nil
	}
	if err := readBin(r, msg); err != nil {
		return fmt.Errorf("parsing %s: %v", messageName, err)
	}
	return nil
}

// WriteBinChunked writes msg to binary format.
func WriteBinChunked(w io.Writer, msg any, messageName string) error {
	if c, ok := msg.(Chunked); ok {
		for chunk := range c.Chunks() {
			if chunk == nil {
				continue
			}
			if err, ok := chunk.(chunkError); ok {
				return fmt.Errorf("serializing %s: %v", messageName, err)
			}
			if err := binary.Write(w, Endian, chunk); err != nil {
				return fmt.Errorf("serializing %s: %v", messageName, err)
			}
		}
		return nil
	}
	if err := binary.Write(w, Endian, msg); err != nil {
		return fmt.Errorf("serializing %s: %v", messageName, err)
	}
	return nil
}

// ParseMsg parses a complete SBF packet.
func ParseMsg(packet string) (*Block, error) {
	n := len(packet)
	if n < PacketMinLen {
		return nil, fmt.Errorf("SBF message too short (length %d bytes)", n)
	}
	if packet[0] != Sync1 || packet[1] != Sync2 {
		return nil, fmt.Errorf("SBF message has bad sync")
	}
	length := int(Endian.Uint16([]byte(packet[6:8])))
	if length != n {
		return nil, fmt.Errorf("SBF message length mismatch: header %d, packet %d", length, n)
	}
	if length <= HeaderLen || length%4 != 0 {
		return nil, fmt.Errorf("SBF message has bad length %d", length)
	}
	got := Endian.Uint16([]byte(packet[2:4]))
	want := CRC16([]byte(packet[4:]))
	if got != want {
		return nil, fmt.Errorf("SBF checksum failed: got %04x, want %04x", got, want)
	}
	id := MsgID(Endian.Uint16([]byte(packet[4:6])))
	blockNumber, rev := id.Unpack()
	body := packet[HeaderLen:]
	if len(body) < 6 {
		return nil, fmt.Errorf("SBF-%s body too short (%d bytes)", id, len(body))
	}
	ts := TimeStamp{
		TOW: Endian.Uint32([]byte(body[:4])),
		WNc: Endian.Uint16([]byte(body[4:6])),
	}
	paramPayload := body[6:]
	ctor := blockMap[blockNumber]
	if ctor != nil {
		p := ctor()
		if ps, ok := p.(payloadSizer); ok {
			ps.setPayloadLen(len(paramPayload))
			defer ps.clearPayloadLen()
		}
		if err := ReadBinChunked(strings.NewReader(paramPayload), p, id.String()); err != nil {
			return nil, err
		}
		return &Block{Rev: rev, TimeStamp: ts, Params: p}, nil
	}
	b := &Block{
		Rev:       rev,
		TimeStamp: ts,
		Params:    &UnknownParams{Number: blockNumber, Payload: body[6:]},
	}
	return b, nil
}

// Serialize serializes an SBF block to a complete packet.
func Serialize(b *Block) ([]byte, error) {
	buf := new(strings.Builder)
	if err := binary.Write(buf, Endian, b.TimeStamp); err != nil {
		return nil, err
	}
	if u, ok := b.Params.(*UnknownParams); ok {
		buf.WriteString(u.Payload)
	} else if err := WriteBinChunked(buf, b.Params, b.ID().String()); err != nil {
		return nil, err
	}
	return PackMsg(b.ID(), []byte(buf.String()))
}

// PackMsg creates a complete SBF packet from an ID and body payload.
func PackMsg(mid MsgID, payload []byte) ([]byte, error) {
	padded := append([]byte(nil), payload...)
	for (HeaderLen+len(padded))%4 != 0 {
		padded = append(padded, 0)
	}
	if HeaderLen+len(padded) > 0xFFFF {
		return nil, fmt.Errorf("SBF-%s payload too long (%d bytes)", mid, len(padded))
	}
	packet := []byte{Sync1, Sync2, 0, 0, 0, 0, 0, 0}
	Endian.PutUint16(packet[4:6], uint16(mid))
	Endian.PutUint16(packet[6:8], uint16(HeaderLen+len(padded)))
	packet = append(packet, padded...)
	Endian.PutUint16(packet[2:4], CRC16(packet[4:]))
	return packet, nil
}

// PacketMsgID returns the masked block number of a packet.
func PacketMsgID[B ~string | ~[]byte](packet B) MsgID {
	return MsgID(Endian.Uint16([]byte{packet[4], packet[5]}) & 0x1FFF)
}
