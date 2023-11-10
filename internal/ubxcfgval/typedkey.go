package ubxcfgval

//go:generate go run mkconsts.go

import "math"

type KeyU uint32
type KeyL uint32
type KeyI uint32
type KeyR uint32

type KeyE[E ~uint8] uint32

type TypedKey[T comparable] interface {
	Key() Key
	Typed(raw uint64) T
	Raw(t T) uint64
}

var _ TypedKey[uint64] = KeyU(0)
var _ TypedKey[bool] = KeyL(0)
var _ TypedKey[int64] = KeyI(0)
var _ TypedKey[float64] = KeyR(0)
var _ TypedKey[uint8] = KeyE[uint8](0)

func MapSet[T comparable](m Map, k TypedKey[T], v T) {
	m[k.Key()] = k.Raw(v)
}

func MapGet[T comparable](m Map, k TypedKey[T]) (t T, ok bool) {
	raw, ok := m[k.Key()]
	if ok {
		t = k.Typed(raw)
	}
	return
}

func AddItem[T comparable](items *[]Item, k TypedKey[T], v T) {
	*items = append(*items, Item{k.Key(), k.Raw(v)})
}

func AddKey[T comparable](keys *[]Key, k TypedKey[T]) {
	*keys = append(*keys, k.Key())
}

func (k KeyU) Key() Key { return Key(k) }

func (k KeyU) Typed(raw uint64) uint64 {
	return raw
}

func (k KeyU) Raw(u uint64) uint64 {
	return u
}

func (k KeyL) Key() Key { return Key(k) }

func (k KeyL) Typed(raw uint64) bool {
	return raw != 0
}

func (k KeyL) Raw(b bool) uint64 {
	if b {
		return 1
	} else {
		return 0
	}
}

func (k KeyI) Key() Key { return Key(k) }

func (k KeyI) Typed(raw uint64) int64 {
	switch Key(k).valueBits() {
	case 8:
		return int64(int8(uint8(raw)))
	case 16:
		return int64(int16(uint16(raw)))
	case 32:
		return int64(int32(uint32(raw)))
	case 64:
		return int64(raw)
	default:
		panic("invalid signed int value size")
	}
}

func (k KeyI) Raw(i int64) uint64 {
	switch Key(k).valueBits() {
	case 8:
		return uint64(uint8(i))
	case 16:
		return uint64(uint16(i))
	case 32:
		return uint64(uint32(i))
	case 64:
		return uint64(i)
	default:
		panic("invalid signed int value size")
	}
}

func (k KeyR) Key() Key { return Key(k) }

func (k KeyR) Typed(raw uint64) float64 {
	switch Key(k).valueBits() {
	case 32:
		return float64(math.Float32frombits(uint32(raw)))
	case 64:
		return math.Float64frombits(raw)
	default:
		panic("invalid float value size")
	}
}

func (k KeyR) Raw(x float64) uint64 {
	switch Key(k).valueBits() {
	case 32:
		return uint64(math.Float32bits(float32(x)))
	case 64:
		return math.Float64bits(x)
	default:
		panic("invalid float value size")
	}
}

func (k KeyE[E]) Key() Key { return Key(k) }

func (k KeyE[E]) Typed(v uint64) E {
	return E(v)
}

func (k KeyE[E]) Raw(e E) uint64 {
	return uint64(e)
}
