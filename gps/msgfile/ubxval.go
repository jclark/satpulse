package msgfile

import (
	"encoding/binary"
	"fmt"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
	"github.com/jclark/satpulse/gps/lib/ubxcfgval"
)

// UBXValMsg represents a [[ubxval]] entry. It emits a UBX-CFG-VALSET
// packet with one or more typed key/value items. The layer used by the
// emitted packet is supplied at encoding time, not stored on the message.
type UBXValMsg struct {
	Keys   []uint32 `toml:"keys"`
	Types  string   `toml:"types"`
	Values []any    `toml:"values"`
	MsgCommon
}

func (um *UBXValMsg) getTag() string { return *um.Tag }

// analyzeRequest treats the emitted packet as a regular UBX-CFG-VALSET write
// for response correlation.
func (um *UBXValMsg) analyzeRequest(data string) requestAnalysis {
	corr := ubxMsgIDString(ubxbin.PacketMsgId(data))
	return requestAnalysis{
		ackTag:       gpsreg.TagUBX,
		ackCorrelate: corr,
		expectAck:    ExpectAckOrNak,
		expectData:   expectDataNone,
	}
}

// ubxValLayers returns the CFG-VALSET layer mask for the given save flag.
func ubxValLayers(save bool) ubxbin.CfgValsetLayer {
	if save {
		return ubxbin.CfgValsetLayerRAM | ubxbin.CfgValsetLayerBBR | ubxbin.CfgValsetLayerFlash
	}
	return ubxbin.CfgValsetLayerRAM
}

// toRawWithLayer encodes the message as a UBX-CFG-VALSET packet using the
// given layer mask.
func (um *UBXValMsg) toRawWithLayer(layer ubxbin.CfgValsetLayer) (RawMsg, error) {
	pkt, err := newUBXValRaw(um.Keys, um.Types, um.Values, layer)
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

// newUBXValRaw builds a UBX-CFG-VALSET packet for the given keys, type
// specifiers, and values.
func newUBXValRaw(keys []uint32, typesStr string, values []any, layer ubxbin.CfgValsetLayer) ([]byte, error) {
	types, err := parseTypes(typesStr)
	if err != nil {
		return nil, err
	}
	if len(types) != len(keys) || len(types) != len(values) {
		return nil, fmt.Errorf("ubxval: %d keys, %d types, %d values must match",
			len(keys), len(types), len(values))
	}
	items := make([]ubxcfgval.Item, len(keys))
	for i, k := range keys {
		key := ubxcfgval.Key(k)
		keyBytes := key.NValueBytes()
		if keyBytes == 0 {
			return nil, fmt.Errorf("ubxval item %d: key 0x%08x has no recognized value size", i+1, k)
		}
		valBytes, err := encodeValue(types[i], values[i], ubxbin.Endian)
		if err != nil {
			return nil, fmt.Errorf("ubxval item %d: %w", i+1, err)
		}
		if len(valBytes) != keyBytes {
			return nil, fmt.Errorf("ubxval item %d: type %s encodes %d bytes but key 0x%08x expects %d",
				i+1, typeSpecName(types[i]), len(valBytes), k, keyBytes)
		}
		var buf [8]byte
		copy(buf[:], valBytes)
		items[i] = ubxcfgval.Item{Key: key, Value: binary.LittleEndian.Uint64(buf[:])}
	}
	cfgData, err := ubxcfgval.MarshalItems(items)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 4+len(cfgData))
	payload[0] = byte(ubxbin.CfgValsetVersionNoTransaction)
	payload[1] = byte(layer)
	payload[2] = byte(ubxbin.CfgValTransactionNone)
	// payload[3] reserved
	copy(payload[4:], cfgData)
	return ubxbin.PackMsg(ubxbin.CfgValsetID, payload)
}

// toRawUBXValMsgs walks ubxval messages, encoding each with the layer
// implied by save. It mirrors toRawMsgs but threads the layer through.
func toRawUBXValMsgs(msgs []UBXValMsg, save bool) ([]RawMsg, error) {
	layer := ubxValLayers(save)
	result := make([]RawMsg, len(msgs))
	tagCount := make(map[string]int)
	for i := range msgs {
		m := &msgs[i]
		tag := m.getTag()
		idx := tagCount[tag]
		tagCount[tag]++
		rm, err := m.toRawWithLayer(layer)
		if err != nil {
			return nil, fmt.Errorf("message %d with tag %q: %w", idx+1, tag, err)
		}
		rm.Index = idx
		rm.Tag = tag
		rm.source = m
		result[i] = rm
	}
	for i := range result {
		result[i].Count = tagCount[result[i].Tag]
	}
	return result, nil
}

func typeSpecName(t typeSpec) string {
	for name, ts := range typeMap {
		if ts == t {
			return name
		}
	}
	return fmt.Sprintf("typeSpec(%d)", int(t))
}
