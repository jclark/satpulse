package ubxcfgval

//go:generate go run mkdfltschema.go

import (
	"fmt"
	"math"
)

type Desc interface {
	key() Key
	MarshalValue(v any) (uint64, error)
	UnmarshalValue(data uint64) (any, error)
}

type U uint32
type I uint32
type L uint32
type R uint32

type EDesc struct {
	k      uint32
	values []string
}

func (d *EDesc) key() Key {
	return Key(d.k)
}

func E(k uint32, values ...string) *EDesc {
	return &EDesc{k, values}
}

func (d U) key() Key {
	return Key(d)
}

func (d I) key() Key {
	return Key(d)
}

func (d R) key() Key {
	return Key(d)
}

func (d L) key() Key {
	return Key(d)
}

type NameDesc struct {
	groupName string
	itemName  string
	d         Desc
}

type Schema struct {
	groups map[string]map[string]Desc
	keys   map[Key]NameDesc
}

func GetDfltSchema() *Schema {
	return dfltSchema
}

func (s *Schema) AddGroup(groupName string, group map[string]Desc) *Schema {
	// Copy existing groups
	groups := make(map[string]map[string]Desc)
	for k, v := range s.groups {
		groups[k] = v
	}
	groups[groupName] = group
	
	// Copy existing keys
	keys := make(map[Key]NameDesc)
	for k, v := range s.keys {
		keys[k] = v
	}
	
	// Add keys from new group
	for itemName, d := range group {
		keys[d.key()] = NameDesc{groupName, itemName, d}
	}
	
	return &Schema{groups, keys}
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

func makeKeys(m map[string]map[string]Desc) (map[Key]NameDesc, error) {
	keys := make(map[Key]NameDesc)
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

func (s *Schema) UnmarshalItems(data []byte) (map[string]map[string]any, map[Key]uint64, error) {
	cfg := make(map[string]map[string]any)
	unknown := make(map[Key]uint64)
	items, err := UnmarshalItems(data)
	if err != nil {
		return nil, nil, err
	}
	for _, it := range items {
		nd, ok := s.keys[it.Key]
		var v any
		if ok {
			var err error
			v, err = nd.d.UnmarshalValue(it.Value)
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
			unknown[it.Key] = it.Value
		}
	}
	return cfg, unknown, nil
}


func (s *Schema) UnmarshalItemsFlat(data []byte) ([]string, []any, error) {
	items, err := UnmarshalItems(data)
	if err != nil {
		return nil, nil, err
	}
	keys := make([]string, len(items))
	values := make([]any, len(items))
	for i, it := range items {
		nd, key, ok := s.decodeKey(it.Key)
		var value any
		if ok {
			var err error
			value, err = nd.d.UnmarshalValue(it.Value)
			if err != nil {
				ok = false
			}
		}
		if !ok {
			value = it.Value
		}
		keys[i] = key
		values[i] = value
	}
	return keys, values, nil
}

func (s *Schema) UnmarshalKeysFlat(data []byte) ([]string, error) {
	keyList, err := UnmarshalKeys(data)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(keyList))
	for i, k := range keyList {
		_, key, _ := s.decodeKey(k)
		keys[i] = key
	}
	return keys, nil
}

func (s *Schema) decodeKey(k Key) (NameDesc, string, bool) {
	nd, ok := s.keys[k]
	if !ok {
		return NameDesc{}, fmt.Sprintf("0x%08x", uint32(k)), false
	}
	return nd, fmt.Sprintf("CFG-%s-%s", nd.groupName, nd.itemName), true
}

func (s *Schema) Marshal(cfg map[string]map[string]any) ([]byte, error) {
	items, err := s.Compile(cfg)
	if err != nil {
		return nil, err
	}
	return MarshalItems(items)
}

func (s *Schema) Compile(cfg map[string]map[string]any) ([]Item, error) {
	items := make([]Item, 0)
	for g, v := range cfg {
		desc, ok := s.groups[g]
		if !ok {
			return nil, fmt.Errorf("unknown group %s", g)
		}
		g, err := marshalGroup(desc, v)
		if err != nil {
			return nil, err
		}
		items = append(items, g...)
	}
	return items, nil
}

func marshalGroup(desc map[string]Desc, cfg map[string]any) ([]Item, error) {
	var items []Item
	for s, v := range cfg {
		d, ok := desc[s]
		if !ok {
			return nil, fmt.Errorf("unknown key %s", s)
		}
		val, err := d.MarshalValue(v)
		if err != nil {
			return nil, err
		}
		items = append(items, Item{d.key(), val})
	}
	return items, nil
}

func (d *EDesc) UnmarshalValue(data uint64) (any, error) {
	n := uint64(0)
	switch d.key().NValueBytes() {
	case 1:
		n = uint64(uint8(data))
	case 2:
		n = uint64(uint16(data))
	case 4:
		n = uint64(uint32(data))
	case 8:
		n = data
	}
	if n >= uint64(len(d.values)) {
		// XXX maybe return uintN here
		return nil, fmt.Errorf("invalid value %d for 0x%x", n, d.key())
	}
	return d.values[n], nil
}

func (d *EDesc) MarshalValue(v any) (uint64, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("cannot marshal %T to enum", v)
	}
	for i, value := range d.values {
		if s == value {
			return d.key().marshalUnsigned(uint64(i))
		}
	}
	return 0, fmt.Errorf("invalid value %s for enum", s)
}

func (d L) UnmarshalValue(data uint64) (any, error) {
	if (data & 1) == 0 {
		return false, nil
	}
	return true, nil
}

func (d L) MarshalValue(v any) (uint64, error) {
	b, ok := v.(bool)
	if !ok {
		return 0, fmt.Errorf("cannot marshal %T to bool", v)
	}
	return uint64(boolToByte(b)), nil
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func (d R) UnmarshalValue(data uint64) (any, error) {
	switch d.key().NValueBytes() {
	case 4:
		bits := uint32(data)
		return math.Float32frombits(bits), nil
	}
	return math.Float64frombits(data), nil
}

func (d R) MarshalValue(v any) (uint64, error) {
	switch d.key().valueBits() {
	case 64:
		f, ok := v.(float64)
		if !ok {
			return 0, fmt.Errorf("cannot marshal %T to float64", v)
		}
		return math.Float64bits(f), nil
	case 32:
		f, ok := v.(float32)
		if !ok {
			return 0, fmt.Errorf("cannot marshal %T to float32", v)
		}

		return uint64(math.Float32bits(f)), nil
	default:
		panic(fmt.Sprintf("invalid key ID 0x%x for float type", d.key()))
	}
}

func (d U) UnmarshalValue(data uint64) (any, error) {
	switch d.key().NValueBytes() {
	case 1:
		return uint8(data), nil
	case 2:
		return uint16(data), nil
	case 4:
		return uint32(data), nil
	default:
		return uint64(data), nil
	}
}

func (d U) MarshalValue(v any) (uint64, error) {
	n, ok := unsignedWiden(v)
	if !ok {
		s, ok := signedWiden(v)
		if !ok {
			return 0, fmt.Errorf("cannot marshal %T to unsigned integer", v)
		}
		if s < 0 {
			return 0, fmt.Errorf("cannot marshal negative integer %d to unsigned integer", s)
		}
		n = uint64(s)
	}
	bits := d.key().valueBits()
	if bits < 64 && n >= 1<<bits {
		return 0, fmt.Errorf("value %d too large for U%d", n, bits)
	}
	return d.key().marshalUnsigned(n)
}

func (d I) UnmarshalValue(data uint64) (any, error) {
	switch d.key().NValueBytes() {
	case 1:
		return int8(uint8(data)), nil
	case 2:
		return int16(uint16(data)), nil
	case 4:
		return int32(uint32(data)), nil
	default:
		return int64(data), nil
	}
}

func (d I) MarshalValue(v any) (uint64, error) {
	n, ok := signedWiden(v)
	if !ok {
		s, ok := unsignedWiden(v)
		if !ok {
			return 0, fmt.Errorf("cannot marshal %T to signed integer", v)
		}
		if s > math.MaxInt64 {
			return 0, fmt.Errorf("cannot marshal unsigned integer %d to signed integer", s)
		}
		n = int64(s)
	}
	bits := d.key().valueBits()
	if bits < 64 && (n >= 1<<(bits-1) || n < -1<<(bits-1)) {
		return 0, fmt.Errorf("value %d does not fit in I%d", n, bits)
	}
	return d.key().marshalUnsigned(uint64(n))
}

func (k Key) marshalUnsigned(n uint64) (uint64, error) {
	switch k.valueBits() {
	case 8:
		return uint64(uint8(n)), nil
	case 16:
		return uint64(uint16(n)), nil
	case 32:
		return uint64(uint32(n)), nil
	default:
		return n, nil
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
