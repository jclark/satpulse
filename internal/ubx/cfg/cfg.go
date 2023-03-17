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
	UnmarshalValue(data []byte) (any, error)
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

type NameDesc struct {
	groupName string
	itemName  string
	d         Desc
}

type Schema struct {
	groups map[string]map[string]Desc
	keys   map[uint32]NameDesc
}

func GetSchema() *Schema {
	return schema
}

func MustNewSchema(groups map[string]map[string]Desc) *Schema {
	s, err := NewSchema(groups)
	if err != nil {
		panic(err)
	}
	return s
}

func NewSchema(groups map[string]map[string]Desc) (*Schema, error) {
	keys, err := makeKeys(groups)
	if err != nil {
		return nil, err
	}
	return &Schema{groups, keys}, nil
}

func makeKeys(m map[string]map[string]Desc) (map[uint32]NameDesc, error) {
	keys := make(map[uint32]NameDesc)
	for groupName, v := range m {
		for itemName, d := range v {
			keys[d.key()] = NameDesc{groupName, itemName, d}
		}
	}
	return keys, nil
}

func (s *Schema) MustMarshal(cfg map[string]map[string]any) []byte {
	b, err := s.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	return b
}

func (s *Schema) Unmarshal(data []byte) (map[string]map[string]any, map[uint32][]byte, error) {
	cfg := make(map[string]map[string]any)
	unknown := make(map[uint32][]byte)
	for len(data) > 4 {
		k := binary.LittleEndian.Uint32(data)
		data = data[4:]
		nBytes := valueBytes(k)
		if len(data) < nBytes {
			return nil, nil, fmt.Errorf("invalid data length for key 0x%x", k)
		}
		itemData := data[0:nBytes]
		data = data[nBytes:]
		nd, ok := s.keys[k]
		var v any
		if ok {
			var err error
			v, err = nd.d.UnmarshalValue(itemData)
			if err != nil {
				ok = false
			}
		}
		if ok {
			g := cfg[nd.groupName]
			if g == nil {
				g = map[string]any{}
				cfg[nd.groupName] = g
			}
			g[nd.itemName] = v
		} else {
			unknown[k] = append([]byte{}, itemData...)
		}
	}
	if len(data) > 0 {
		return nil, nil, fmt.Errorf("leftover data")
	}
	return cfg, unknown, nil
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

func (d *EDesc) UnmarshalValue(data []byte) (any, error) {
	n := uint64(0)
	switch len(data) {
	case 1:
		n = uint64(data[0])
	case 2:
		n = uint64(binary.LittleEndian.Uint16(data))
	case 4:
		n = uint64(binary.LittleEndian.Uint32(data))
	case 8:
		n = uint64(binary.LittleEndian.Uint64(data))
	}
	if n >= uint64(len(d.values)) {
		// XXX maybe return uintN here
		return nil, fmt.Errorf("invalid value %d for 0x%x", n, d.key())
	}
	return d.values[n], nil
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

func (d L) UnmarshalValue(data []byte) (any, error) {
	if data[0] == 0 {
		return false, nil
	}
	return true, nil
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

func (d R) UnmarshalValue(data []byte) (any, error) {
	switch len(data) {
	case 4:
		bits := binary.LittleEndian.Uint32(data)
		return math.Float32frombits(bits), nil
	}
	bits := binary.LittleEndian.Uint64(data)
	return math.Float64frombits(bits), nil
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

func (d U) UnmarshalValue(data []byte) (any, error) {
	switch len(data) {
	case 1:
		return data[0], nil
	case 2:
		return binary.LittleEndian.Uint16(data), nil
	case 4:
		return binary.LittleEndian.Uint32(data), nil
	default:
		return binary.LittleEndian.Uint64(data), nil
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

func (d I) UnmarshalValue(data []byte) (any, error) {
	switch len(data) {
	case 1:
		return int8(data[0]), nil
	case 2:
		return int16(binary.LittleEndian.Uint16(data)), nil
	case 4:
		return int32(binary.LittleEndian.Uint32(data)), nil
	default:
		return int64(binary.LittleEndian.Uint64(data)), nil
	}
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

func valueBytes(k uint32) int {
	return (valueBits(k) + 7) / 8
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
