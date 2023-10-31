package ubxcfgval

import (
	"encoding"
	"encoding/binary"
	"fmt"
	"sort"
)

type Key uint32
type Map map[Key]uint64

var _ encoding.BinaryUnmarshaler = (*Map)(nil)
var _ encoding.BinaryMarshaler = (*Map)(nil)

func (mp *Map) MarshalBinary() ([]byte, error) {
	m := *mp
	keys := make([]Key, 0, len(m))
	nBytes := 0
	for k := range m {
		keys = append(keys, k)
		nBytes += 4 + k.nValueBytes()
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	bytes := make([]byte, 0, nBytes)
	valBuf := make([]byte, 8)
	for _, k := range keys {
		kb, _ := k.MarshalBinary()
		bytes = append(bytes, kb...)
		binary.LittleEndian.PutUint64(valBuf, m[k])
		bytes = append(bytes, valBuf[0:k.nValueBytes()]...)
	}
	return bytes, nil
}

func (mp *Map) UnmarshalBinary(data []byte) error {
	m := make(Map)
	for len(data) > 4 {
		k := Key(binary.LittleEndian.Uint32(data))
		data = data[4:]
		nBytes := k.nValueBytes()
		if len(data) < nBytes {
			return fmt.Errorf("invalid data length for key 0x%x", k)
		}
		// this is not the most efficient way to do things, but this won't be a hotspot
		valBytes := make([]byte, 8)
		copy(valBytes, data[0:nBytes])
		m[k] = binary.LittleEndian.Uint64(valBytes)
		data = data[nBytes:]
	}
	if len(data) > 0 {
		return fmt.Errorf("leftover data")
	}
	*mp = m
	return nil
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
