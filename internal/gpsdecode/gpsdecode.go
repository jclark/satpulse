package gpsdecode

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/jclark/satpulse/internal/asbin"
	casicbin "github.com/jclark/satpulse/internal/casbin"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
	"github.com/jclark/satpulse/internal/novmsg"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/ubxbin"
	"github.com/jclark/satpulse/internal/ubxcfgval"
	"github.com/jclark/satpulse/internal/uncmsg"
)

var (
	ErrUnknownFormat = errors.New("unknown packet format")
	ErrInvalidPacket = errors.New("invalid packet structure")
	ErrUnknownMsg    = errors.New("unknown message type")
)

// ChecksumError indicates a packet has an invalid checksum.
type ChecksumError struct {
	InPacket []byte // checksum bytes extracted from the packet
	Computed []byte // checksum bytes we computed
}

func (e *ChecksumError) Error() string {
	return fmt.Sprintf("checksum mismatch: in packet %x, computed %x", e.InPacket, e.Computed)
}

// DecodeResult holds decoded packet fields in a fixed order for JSON serialization.
type DecodeResult struct {
	Payload any `json:"payload,omitempty"`
	Header  any `json:"header,omitempty"`
	CfgData any `json:"cfgData,omitempty"`
}

var cfgSchema = ubxcfgval.NewSchemaWithMsgout(ubxcfgval.GetDfltSchema())

// Decode parses a packet and returns the PacketFormat and decoded fields.
// It uses scan.LooksLike to identify the format and gpsprot.IsValidPacket to validate.
// Returns PacketFormat so caller can get both Tag() and MsgID(data).
func Decode(pktFormats []gpsprot.PacketFormat, data []byte, out bool) (gpsprot.PacketFormat, *DecodeResult, error) {
	pf := scan.LooksLike(pktFormats, data)
	if pf == nil {
		return nil, nil, ErrUnknownFormat
	}
	if !gpsprot.IsValidPacket(pf, data) {
		return nil, nil, ErrInvalidPacket
	}
	// Validate checksum
	inPacket := pf.ExtractChecksum(data)
	computed := pf.ComputeChecksum(data)
	if !bytes.Equal(inPacket, computed) {
		// Check alternate checksum for formats that support it
		if alt, ok := pf.(gpsprot.AltChecksumPacketFormat); ok {
			if bytes.Equal(inPacket, alt.ComputeAltChecksum(data)) {
				goto checksumOK
			}
		}
		return pf, nil, &ChecksumError{InPacket: inPacket, Computed: computed}
	}
checksumOK:
	switch pf.Tag() {
	case gpsreg.TagUBX:
		r, err := ubxbinDecode(data, out)
		return pf, r, err
	case gpsreg.TagCASICBin:
		r, err := casicDecode(data)
		return pf, r, err
	case gpsreg.TagAllystarBin:
		r, err := asbinDecode(data)
		return pf, r, err
	case gpsreg.TagUnicoreBin:
		r, err := uncbinDecode(data)
		return pf, r, err
	case gpsreg.TagNovAtelBin:
		r, err := novbinDecode(data)
		return pf, r, err
	default:
		return pf, nil, ErrUnknownFormat
	}
}

func ubxbinDecode(data []byte, out bool) (*DecodeResult, error) {
	msg, err := ubxbin.ParseMsg(string(data))
	if err != nil {
		return nil, err
	}
	if _, isUnknown := msg.(*ubxbin.UnknownMsg); isUnknown {
		return nil, ErrUnknownMsg
	}
	result := &DecodeResult{Payload: msg}
	cfgData, isKeys := msgCfgData(msg, out)
	if cfgData != nil {
		var encoded any
		if isKeys {
			encoded, err = encodeCfgDataKeys(cfgData)
		} else {
			encoded, err = encodeCfgDataItems(cfgData)
		}
		if err == nil {
			result.CfgData = encoded
		}
	}
	return result, nil
}

// msgCfgData extracts CfgData from CFG-VAL* messages.
// Returns the data and whether it contains keys only (vs key-value items).
func msgCfgData(msg ubxbin.Msg, out bool) ([]byte, bool) {
	switch m := msg.(type) {
	case *ubxbin.CfgValget:
		// Request (out=true) has keys, response (out=false) has items
		return m.CfgData, out
	case *ubxbin.CfgValset:
		return m.CfgData, false
	case *ubxbin.CfgValdel:
		return m.CfgData, true
	default:
		return nil, false
	}
}

func encodeCfgDataItems(cfgData []byte) (any, error) {
	keys, values, err := cfgSchema.UnmarshalItemsFlat(cfgData)
	if err != nil {
		return nil, err
	}
	items := make([][2]any, len(keys))
	for i := range keys {
		items[i] = [2]any{keys[i], values[i]}
	}
	return items, nil
}

func encodeCfgDataKeys(cfgData []byte) (any, error) {
	return cfgSchema.UnmarshalKeysFlat(cfgData)
}

func casicDecode(data []byte) (*DecodeResult, error) {
	msg, err := casicbin.ParseMsg(string(data))
	if err != nil {
		return nil, err
	}
	if _, isUnknown := msg.(*casicbin.UnknownMsg); isUnknown {
		return nil, ErrUnknownMsg
	}
	return &DecodeResult{Payload: msg}, nil
}

func asbinDecode(data []byte) (*DecodeResult, error) {
	msg, err := asbin.ParseMsg(string(data))
	if err != nil {
		return nil, err
	}
	if _, isUnknown := msg.(*asbin.UnknownMsg); isUnknown {
		return nil, ErrUnknownMsg
	}
	return &DecodeResult{Payload: msg}, nil
}

func uncbinDecode(data []byte) (*DecodeResult, error) {
	msg, err := uncmsg.ParseBinMsg(data)
	if err != nil {
		return nil, err
	}
	if _, isUnknown := msg.Body.(*uncmsg.UnknownBinMsgBody); isUnknown {
		return nil, ErrUnknownMsg
	}
	return &DecodeResult{
		Payload: msg.Body,
		Header:  msg.Hdr,
	}, nil
}

func novbinDecode(data []byte) (*DecodeResult, error) {
	msg, err := novmsg.ParseBinMsg(data)
	if err != nil {
		return nil, err
	}
	if _, isUnknown := msg.Body.(*novmsg.UnknownBinMsgBody); isUnknown {
		return nil, ErrUnknownMsg
	}
	return &DecodeResult{
		Payload: msg.Body,
		Header:  msg.Hdr,
	}, nil
}
