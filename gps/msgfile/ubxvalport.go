package msgfile

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// UBXValPortMsg represents a [[ubxvalport]] entry. It emits a
// UBX-CFG-VALSET packet for a port-dependent configuration item with a
// one-byte value. The port is chosen at encoding time from the --port
// command-line argument.
type UBXValPortMsg struct {
	Key   UBXValPortKeys `toml:"key"`
	Value uint8          `toml:"value"`
	MsgCommon
}

// UBXValPortKeys holds the optional per-port key IDs.
type UBXValPortKeys struct {
	I2C   opt.Val[uint32] `toml:"i2c,omitzero"`
	UART1 opt.Val[uint32] `toml:"uart1,omitzero"`
	UART2 opt.Val[uint32] `toml:"uart2,omitzero"`
	USB   opt.Val[uint32] `toml:"usb,omitzero"`
	SPI   opt.Val[uint32] `toml:"spi,omitzero"`
}

func (um *UBXValPortMsg) getTag() string { return *um.Tag }

// analyzeRequest treats the emitted packet as a regular UBX-CFG-VALSET write
// for response correlation.
func (um *UBXValPortMsg) analyzeRequest(data string) requestAnalysis {
	return requestAnalysis{
		ackTag:       gpsreg.TagUBX,
		ackCorrelate: ubxMsgIDString(ubxbin.PacketMsgId(data)),
		expectAck:    ExpectAckOrNak,
		expectData:   expectDataNone,
	}
}

// ubxPorts enumerates the five u-blox output ports in offset order. Each
// entry pairs the lower-case TOML port name with its CFG-..._<PORT> offset
// and an accessor for the corresponding UBXValPortKeys field.
var ubxPorts = [...]struct {
	name string
	get  func(*UBXValPortKeys) opt.Val[uint32]
}{
	{"i2c", func(k *UBXValPortKeys) opt.Val[uint32] { return k.I2C }},
	{"uart1", func(k *UBXValPortKeys) opt.Val[uint32] { return k.UART1 }},
	{"uart2", func(k *UBXValPortKeys) opt.Val[uint32] { return k.UART2 }},
	{"usb", func(k *UBXValPortKeys) opt.Val[uint32] { return k.USB }},
	{"spi", func(k *UBXValPortKeys) opt.Val[uint32] { return k.SPI }},
}

// ubxPortOffset returns the standard u-blox port offset for the given port
// name. Names are matched case-insensitively.
func ubxPortOffset(port string) (uint32, error) {
	port = strings.ToLower(port)
	for i, p := range ubxPorts {
		if p.name == port {
			return uint32(i), nil
		}
	}
	return 0, fmt.Errorf("unknown u-blox port %q", port)
}

// Known one-byte port-dependent CFG families that follow the standard
// i2c/uart1/uart2/usb/spi offset pattern.
const (
	ubxCfgMsgoutGroup = 0x20910000
	ubxCfgInfmsgGroup = 0x20920000
)

func isInferablePortGroup(key uint32) bool {
	switch key & 0xffff0000 {
	case ubxCfgMsgoutGroup, ubxCfgInfmsgGroup:
		return true
	}
	return false
}

type ubxPortSuppliedKey struct {
	offset uint32
	key    uint32
	port   string
}

// suppliedKeys returns the set of port keys explicitly supplied in the
// message file in port-offset order.
func (k *UBXValPortKeys) suppliedKeys() []ubxPortSuppliedKey {
	var out []ubxPortSuppliedKey
	for i, p := range ubxPorts {
		if v := p.get(k); v.IsSet() {
			out = append(out, ubxPortSuppliedKey{uint32(i), v.Get(), p.name})
		}
	}
	return out
}

// resolveKey returns the key for the requested port offset, validating
// supplied keys.
func (k *UBXValPortKeys) resolveKey(targetOffset uint32) (uint32, error) {
	supplied := k.suppliedKeys()
	if len(supplied) == 0 {
		return 0, fmt.Errorf("ubxvalport: at least one key.<port> value must be present")
	}
	if len(supplied) == 5 {
		for _, s := range supplied {
			if s.offset == targetOffset {
				return s.key, nil
			}
		}
		return 0, fmt.Errorf("ubxvalport: missing key for port offset %d", targetOffset)
	}
	for _, s := range supplied {
		if !isInferablePortGroup(s.key) {
			return 0, fmt.Errorf("ubxvalport: key.%s = 0x%08x is not in a port-inferable family (CFG-MSGOUT or CFG-INFMSG); supply all five key.<port> values to bypass inference",
				s.port, s.key)
		}
	}
	base := supplied[0].key - supplied[0].offset
	for _, s := range supplied[1:] {
		if s.key-s.offset != base {
			return 0, fmt.Errorf("ubxvalport: key.%s = 0x%08x is inconsistent with key.%s = 0x%08x under the standard port offset pattern",
				s.port, s.key, supplied[0].port, supplied[0].key)
		}
	}
	for _, s := range supplied {
		if s.offset == targetOffset {
			return s.key, nil
		}
	}
	return base + targetOffset, nil
}

// toRawWithPortLayer encodes the message as a UBX-CFG-VALSET packet for the
// given port and layer mask.
func (um *UBXValPortMsg) toRawWithPortLayer(port string, layer ubxbin.CfgValsetLayer) (RawMsg, error) {
	if port == "" {
		return RawMsg{}, fmt.Errorf("ubxvalport requires --port")
	}
	offset, err := ubxPortOffset(port)
	if err != nil {
		return RawMsg{}, err
	}
	key, err := um.Key.resolveKey(offset)
	if err != nil {
		return RawMsg{}, err
	}
	pkt, err := newUBXValRaw([]uint32{key}, "U1", []any{int64(um.Value)}, layer)
	if err != nil {
		return RawMsg{}, err
	}
	delay, err := um.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := um.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{Bytes: pkt, Delay: delay, WaitLimit: wl, Tag: *um.Tag}, nil
}
