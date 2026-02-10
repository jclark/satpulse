package msgfile

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestPayloadEncode(t *testing.T) {
	tests := []struct {
		name     string
		types    string
		values   []any
		expected []byte
		wantErr  bool
	}{
		{
			name:     "single U1",
			types:    "U1",
			values:   []any{int64(0x42)},
			expected: []byte{0x42},
		},
		{
			name:     "single U2",
			types:    "U2",
			values:   []any{int64(0x1234)},
			expected: []byte{0x34, 0x12}, // little endian
		},
		{
			name:     "single U4",
			types:    "U4",
			values:   []any{int64(0x12345678)},
			expected: []byte{0x78, 0x56, 0x34, 0x12},
		},
		{
			name:     "single I1 positive",
			types:    "I1",
			values:   []any{int64(127)},
			expected: []byte{0x7F},
		},
		{
			name:     "single I1 negative",
			types:    "I1",
			values:   []any{int64(-1)},
			expected: []byte{0xFF},
		},
		{
			name:     "single I2 positive",
			types:    "I2",
			values:   []any{int64(0x1234)},
			expected: []byte{0x34, 0x12},
		},
		{
			name:     "single I2 negative",
			types:    "I2",
			values:   []any{int64(-1)},
			expected: []byte{0xFF, 0xFF},
		},
		{
			name:     "single I4 positive",
			types:    "I4",
			values:   []any{int64(0x12345678)},
			expected: []byte{0x78, 0x56, 0x34, 0x12},
		},
		{
			name:     "single I4 negative",
			types:    "I4",
			values:   []any{int64(-1)},
			expected: []byte{0xFF, 0xFF, 0xFF, 0xFF},
		},
		{
			name:     "multiple values",
			types:    "U1U2U4",
			values:   []any{int64(0x01), int64(0x0203), int64(0x04050607)},
			expected: []byte{0x01, 0x03, 0x02, 0x07, 0x06, 0x05, 0x04},
		},
		{
			name:     "UBX CFG-VALSET example",
			types:    "U1U1U2U4U1",
			values:   []any{int64(0), int64(1), int64(0), int64(0x10320001), int64(1)},
			expected: []byte{0x00, 0x01, 0x00, 0x00, 0x01, 0x00, 0x32, 0x10, 0x01},
		},
		{
			name:     "empty payload",
			types:    "",
			values:   []any{},
			expected: []byte{},
		},
		{
			name:    "type count mismatch",
			types:   "U1U2",
			values:  []any{int64(1)},
			wantErr: true,
		},
		{
			name:    "odd length types string",
			types:   "U1U",
			values:  []any{int64(1), int64(2)},
			wantErr: true,
		},
		{
			name:    "unknown type specifier",
			types:   "U3",
			values:  []any{int64(1)},
			wantErr: true,
		},
		{
			name:    "U1 out of range",
			types:   "U1",
			values:  []any{int64(256)},
			wantErr: true,
		},
		{
			name:    "U2 out of range",
			types:   "U2",
			values:  []any{int64(0x10000)},
			wantErr: true,
		},
		{
			name:    "U4 out of range",
			types:   "U4",
			values:  []any{int64(0x100000000)},
			wantErr: true,
		},
		{
			name:    "I1 out of range positive",
			types:   "I1",
			values:  []any{int64(128)},
			wantErr: true,
		},
		{
			name:    "I1 out of range negative",
			types:   "I1",
			values:  []any{int64(-129)},
			wantErr: true,
		},
		{
			name:    "I2 out of range positive",
			types:   "I2",
			values:  []any{int64(32768)},
			wantErr: true,
		},
		{
			name:    "I2 out of range negative",
			types:   "I2",
			values:  []any{int64(-32769)},
			wantErr: true,
		},
		{
			name:    "negative value for unsigned",
			types:   "U1",
			values:  []any{int64(-1)},
			wantErr: true,
		},
		{
			name:    "wrong value type",
			types:   "U1",
			values:  []any{"string"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Payload{Types: tc.types, Values: tc.values}
			got, err := p.Encode(binary.LittleEndian)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tc.expected) {
				t.Errorf("got %x, expected %x", got, tc.expected)
			}
		})
	}
}

func TestPayloadEncodeFloat(t *testing.T) {
	tests := []struct {
		name     string
		types    string
		values   []any
		expected []byte
	}{
		{
			name:   "R4 float",
			types:  "R4",
			values: []any{float64(1.0)},
			expected: func() []byte {
				b := make([]byte, 4)
				binary.LittleEndian.PutUint32(b, math.Float32bits(1.0))
				return b
			}(),
		},
		{
			name:   "R8 double",
			types:  "R8",
			values: []any{float64(1.0)},
			expected: func() []byte {
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, math.Float64bits(1.0))
				return b
			}(),
		},
		{
			name:   "R4 from int",
			types:  "R4",
			values: []any{int64(42)},
			expected: func() []byte {
				b := make([]byte, 4)
				binary.LittleEndian.PutUint32(b, math.Float32bits(42.0))
				return b
			}(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Payload{Types: tc.types, Values: tc.values}
			got, err := p.Encode(binary.LittleEndian)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, tc.expected) {
				t.Errorf("got %x, expected %x", got, tc.expected)
			}
		})
	}
}

func TestParseTypes(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
		wantErr bool
	}{
		{"", 0, false},
		{"U1", 1, false},
		{"U1U2", 2, false},
		{"U1U2U4I1I2I4R4R8", 8, false},
		{"u1", 1, false},   // lowercase
		{"u1u2", 2, false}, // lowercase
		{"U", 0, true},     // odd length
		{"U3", 0, true},    // unknown type
		{"U1 U2", 0, true}, // space in middle
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			types, err := parseTypes(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(types) != tc.wantLen {
				t.Errorf("got %d types, expected %d", len(types), tc.wantLen)
			}
		})
	}
}
