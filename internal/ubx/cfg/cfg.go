package cfg

//go:generate go run mkschema.go

import (
	"encoding/binary"
	"fmt"
	"math"
)

type Desc interface {
	key() uint32
	MarshalValue(v any) ([]byte, error)
}

type U uint32
type I uint32
type L uint32
type R uint32

type EDesc struct {
	k      uint32
	values []string
}

func E(k uint32, values ...string) *EDesc {
	return &EDesc{k, values}
}

func (d U) key() uint32 {
	return uint32(d)
}

func (d I) key() uint32 {
	return uint32(d)
}

func (d R) key() uint32 {
	return uint32(d)
}

func (d L) key() uint32 {
	return uint32(d)
}

type Schema struct {
	groups map[string]map[string]Desc
}

func (s *Schema) Marshal(cfg map[string]map[string]any) ([]byte, error) {
	var bytes []byte
	for g, v := range cfg {
		desc, ok := s.groups[g]
		if !ok {
			return nil, fmt.Errorf("unknown group %s", g)
		}
		b, err := marshalGroup(desc, v)
		if err != nil {
			return nil, err
		}
		bytes = append(bytes, b...)
	}
	return bytes, nil
}

func marshalGroup(desc map[string]Desc, cfg map[string]any) ([]byte, error) {
	var bytes []byte
	for s, v := range cfg {
		d, ok := desc[s]
		if !ok {
			return nil, fmt.Errorf("unknown key %s", s)
		}
		b, err := marshalKey(d)
		if err != nil {
			return nil, err
		}
		bytes = append(bytes, b...)
		b, err = d.MarshalValue(v)
		if err != nil {
			return nil, err
		}
		bytes = append(bytes, b...)
	}
	return bytes, nil
}

func marshalKey(d Desc) ([]byte, error) {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, d.key())
	return bytes, nil
}

func (d *EDesc) key() uint32 {
	return d.k
}

func (d *EDesc) MarshalValue(v any) ([]byte, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("cannot marshal %T to enum", v)
	}
	for i, value := range d.values {
		if s == value {
			return marshalUnsigned(d.k, uint64(i))
		}
	}
	return nil, fmt.Errorf("invalid value %s for enum", s)
}

func (d L) MarshalValue(v any) ([]byte, error) {
	b, ok := v.(bool)
	if !ok {
		return nil, fmt.Errorf("cannot marshal %T to bool", v)
	}
	return []byte{boolToByte(b)}, nil
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func (d R) MarshalValue(v any) ([]byte, error) {
	switch valueBits(d.key()) {
	case 64:
		f, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("cannot marshal %T to float64", v)
		}
		bytes := make([]byte, 8)
		u8 := math.Float64bits(f)
		binary.LittleEndian.PutUint64(bytes, u8)
		return bytes, nil
	case 32:
		f, ok := v.(float32)
		if !ok {
			return nil, fmt.Errorf("cannot marshal %T to float32", v)
		}
		bytes := make([]byte, 4)
		u4 := math.Float32bits(f)
		binary.LittleEndian.PutUint32(bytes, u4)
		return bytes, nil
	default:
		panic(fmt.Sprintf("invalid key ID 0x%x for float type", d.key()))
	}
}

func (d U) MarshalValue(v any) ([]byte, error) {
	n, ok := unsignedWiden(v)
	if !ok {
		s, ok := signedWiden(v)
		if !ok {
			return nil, fmt.Errorf("cannot marshal %T to unsigned integer", v)
		}
		if s < 0 {
			return nil, fmt.Errorf("cannot marshal negative integer %d to unsigned integer", s)
		}
		n = uint64(s)
	}
	bits := valueBits(d.key())
	if bits < 64 && n >= 1<<bits {
		return nil, fmt.Errorf("value %d too large for U%d", n, bits)
	}
	return marshalUnsigned(d.key(), n)
}

func (d I) MarshalValue(v any) ([]byte, error) {
	n, ok := signedWiden(v)
	if !ok {
		s, ok := unsignedWiden(v)
		if !ok {
			return nil, fmt.Errorf("cannot marshal %T to signed integer", v)
		}
		if s > math.MaxInt64 {
			return nil, fmt.Errorf("cannot marshal unsigned integer %d to signed integer", s)
		}
		n = int64(s)
	}
	bits := valueBits(d.key())
	if bits < 64 && (n >= 1<<(bits-1) || n < -1<<(bits-1)) {
		return nil, fmt.Errorf("value %d does not fit in I%d", n, bits)
	}
	return marshalUnsigned(d.key(), uint64(n))
}

func marshalUnsigned(k uint32, n uint64) ([]byte, error) {
	switch valueBits(k) {
	case 8:
		return []byte{uint8(n)}, nil
	case 16:
		bytes := make([]byte, 2)
		u2 := uint16(n)
		binary.LittleEndian.PutUint16(bytes, u2)
		return bytes, nil
	case 32:
		u4 := uint32(n)
		bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(bytes, u4)
		return bytes, nil
	default:
		bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(bytes, n)
		return bytes, nil
	}
}

func unsignedWiden(v any) (uint64, bool) {
	switch n := v.(type) {
	case uint:
		return uint64(n), true
	case uint8:
		return uint64(n), true
	case uint16:
		return uint64(n), true
	case uint32:
		return uint64(n), true
	case uint64:
		return n, true
	}
	return 0, false
}

func signedWiden(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}

func valueBits(k uint32) int {
	sz := int(k>>28) & 0x0f
	switch sz {
	case 1:
		return 1
	case 2, 3, 4, 5:
		return 2 << sz
	}
	return 0
}
