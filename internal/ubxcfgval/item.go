package ubxcfgval

import (
	"encoding"
	"encoding/binary"
	"fmt"
	"sort"
)

type Key uint32
type Item struct {
	Key   Key
	Value uint64
}

type Map map[Key]uint64

var _ encoding.BinaryUnmarshaler = (*Map)(nil)
var _ encoding.BinaryMarshaler = (*Map)(nil)

func (mp *Map) Items() []Item {
	m := *mp
	items := make([]Item, 0, len(m))
	for k, v := range m {
		items = append(items, Item{k, v})
	}
	return items
}

func SortItems(items []Item) {
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
}

func (mp *Map) AddItems(items []Item) {
	m := *mp
	for _, it := range items {
		m[it.Key] = it.Value
	}
}

func (mp *Map) Contains(k Key) bool {
	_, ok := (*mp)[k]
	return ok
}

func (mp *Map) MarshalBinary() ([]byte, error) {
	items := mp.Items()
	SortItems(items)
	return MarshalItems(items)
}

func (mp *Map) UnmarshalBinary(data []byte) error {
	items, err := UnmarshalItems(data)
	if err != nil {
		return err
	}
	*mp = make(Map)
	mp.AddItems(items)
	return nil
}

func MarshalKeys(keys []Key) []byte {
	bytes := make([]byte, len(keys)*4)
	for i, k := range keys {
		binary.LittleEndian.PutUint32(bytes[i*4:], uint32(k))
	}
	return bytes
}

func MarshalItems(items []Item) ([]byte, error) {
	bytes := make([]byte, len(items)*12)
	i := 0
	for _, it := range items {
		binary.LittleEndian.PutUint32(bytes[i:], uint32(it.Key))
		i += 4
		binary.LittleEndian.PutUint64(bytes[i:], it.Value)
		i += it.Key.nValueBytes()
	}
	return bytes[0:i], nil
}

func UnmarshalItems(data []byte) ([]Item, error) {
	items := make([]Item, 0, len(data)/12)
	for len(data) > 4 {
		k := Key(binary.LittleEndian.Uint32(data))
		data = data[4:]
		nBytes := k.nValueBytes()
		if len(data) < nBytes {
			return nil, fmt.Errorf("invalid data length for key 0x%x", k)
		}
		// this is not the most efficient way to do things, but this won't be a hotspot
		valBytes := make([]byte, 8)
		copy(valBytes, data[0:nBytes])
		items = append(items, Item{k, binary.LittleEndian.Uint64(valBytes)})
		data = data[nBytes:]
	}
	if len(data) > 0 {
		return nil, fmt.Errorf("leftover data")
	}
	return items, nil
}

func (k Key) String() string {
	return fmt.Sprintf("0x%08x", uint32(k))
}

func (k Key) nValueBytes() int {
	return (k.valueBits() + 7) / 8
}

func (k Key) valueBits() int {
	sz := int(k>>28) & 0x0f
	switch sz {
	case 1:
		return 1
	case 2, 3, 4, 5:
		return 2 << sz
	}
	return 0
}

func (k Key) MarshalBinary() ([]byte, error) {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, uint32(k))
	return bytes, nil
}
