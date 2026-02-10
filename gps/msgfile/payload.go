package msgfile

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// Payload specifies how to encode a binary payload.
type Payload struct {
	Types  string `toml:"types"`
	Values []any  `toml:"values"`
}

// Encode encodes the payload using the given byte order.
// Type specifiers: U1, U2, U4, I1, I2, I4, R4, R8
// Each specifier is 2 characters. Values from TOML are int64 for integers
// and float64 for floats; Encode handles conversion and range checking.
func (p *Payload) Encode(endian binary.ByteOrder) ([]byte, error) {
	types, err := parseTypes(p.Types)
	if err != nil {
		return nil, err
	}
	if len(types) != len(p.Values) {
		return nil, fmt.Errorf("payload: %d type specifiers but %d values", len(types), len(p.Values))
	}
	var buf []byte
	for i, t := range types {
		v := p.Values[i]
		b, err := encodeValue(t, v, endian)
		if err != nil {
			return nil, fmt.Errorf("payload value %d: %w", i, err)
		}
		buf = append(buf, b...)
	}
	return buf, nil
}

type typeSpec byte

const (
	typeU1 typeSpec = iota
	typeU2
	typeU4
	typeI1
	typeI2
	typeI4
	typeR4
	typeR8
)

var typeMap = map[string]typeSpec{
	"U1": typeU1,
	"U2": typeU2,
	"U4": typeU4,
	"I1": typeI1,
	"I2": typeI2,
	"I4": typeI4,
	"R4": typeR4,
	"R8": typeR8,
}

func parseTypes(s string) ([]typeSpec, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("payload types: length must be even")
	}
	types := make([]typeSpec, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		spec := strings.ToUpper(s[i : i+2])
		t, ok := typeMap[spec]
		if !ok {
			return nil, fmt.Errorf("payload types: unknown specifier %q", spec)
		}
		types[i/2] = t
	}
	return types, nil
}

func encodeValue(t typeSpec, v any, endian binary.ByteOrder) ([]byte, error) {
	switch t {
	case typeR4, typeR8:
		return encodeFloatValue(t, v, endian)
	default:
		return encodeIntValue(t, v, endian)
	}
}

func encodeIntValue(t typeSpec, v any, endian binary.ByteOrder) ([]byte, error) {
	n, ok := v.(int64)
	if !ok {
		return nil, fmt.Errorf("expected integer, got %T", v)
	}
	switch t {
	case typeU1:
		if n < 0 || n > math.MaxUint8 {
			return nil, fmt.Errorf("value %d out of range for U1", n)
		}
		return []byte{byte(n)}, nil
	case typeU2:
		if n < 0 || n > math.MaxUint16 {
			return nil, fmt.Errorf("value %d out of range for U2", n)
		}
		b := make([]byte, 2)
		endian.PutUint16(b, uint16(n))
		return b, nil
	case typeU4:
		if n < 0 || n > math.MaxUint32 {
			return nil, fmt.Errorf("value %d out of range for U4", n)
		}
		b := make([]byte, 4)
		endian.PutUint32(b, uint32(n))
		return b, nil
	case typeI1:
		if n < math.MinInt8 || n > math.MaxInt8 {
			return nil, fmt.Errorf("value %d out of range for I1", n)
		}
		return []byte{byte(n)}, nil
	case typeI2:
		if n < math.MinInt16 || n > math.MaxInt16 {
			return nil, fmt.Errorf("value %d out of range for I2", n)
		}
		b := make([]byte, 2)
		endian.PutUint16(b, uint16(int16(n)))
		return b, nil
	case typeI4:
		if n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("value %d out of range for I4", n)
		}
		b := make([]byte, 4)
		endian.PutUint32(b, uint32(int32(n)))
		return b, nil
	default:
		panic(fmt.Sprintf("unexpected type specifier %v", t))
	}
}

func encodeFloatValue(t typeSpec, v any, endian binary.ByteOrder) ([]byte, error) {
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case int64:
		f = float64(n)
	default:
		return nil, fmt.Errorf("expected number, got %T", v)
	}
	if t == typeR4 {
		b := make([]byte, 4)
		endian.PutUint32(b, math.Float32bits(float32(f)))
		return b, nil
	}
	b := make([]byte, 8)
	endian.PutUint64(b, math.Float64bits(f))
	return b, nil
}
