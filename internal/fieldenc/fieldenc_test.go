package fieldenc

import (
	"fmt"
	"reflect"
	"testing"
)

// Test struct with basic types
type BasicStruct struct {
	Str    string
	Int32  int32
	Uint64 uint64
	Float  float64
	Bool   bool
}

// Custom type implementing UnmarshalText/MarshalText
type Status uint8

const (
	StatusOK Status = 0
	StatusError Status = 1
)

func (s *Status) UnmarshalText(text []byte) error {
	switch string(text) {
	case "OK":
		*s = StatusOK
	case "ERROR":
		*s = StatusError
	default:
		return fmt.Errorf("unknown status: %s", text)
	}
	return nil
}

func (s Status) MarshalText() ([]byte, error) {
	switch s {
	case StatusOK:
		return []byte("OK"), nil
	case StatusError:
		return []byte("ERROR"), nil
	default:
		return nil, fmt.Errorf("unknown status: %d", s)
	}
}

type CustomStruct struct {
	Name   string
	Status Status
	Value  int
}

type IntStruct struct {
	Num int
}

// Test struct with signed integer types
type SignedIntsStruct struct {
	Int   int
	Int8  int8
	Int16 int16
	Int32 int32
	Int64 int64
}

// Test struct with unsigned integer types
type UnsignedIntsStruct struct {
	Uint   uint
	Uint8  uint8
	Uint16 uint16
	Uint32 uint32
	Uint64 uint64
}

// Test embedded structs
type HeaderFields struct {
	MessageName string
	Port        uint8
	Sequence    uint32
}

type PayloadFields struct {
	Lat float64
	Lon float64
	Alt float32
}

type CompleteMessage struct {
	HeaderFields  // Embedded struct
	PayloadFields // Embedded struct
	Checksum      uint32
}

// Test nested embedded structs
type TimestampFields struct {
	Seconds uint32
	Nanos   uint32
}

type MetadataFields struct {
	TimestampFields // Nested embedded struct
	Source          string
}

type NestedMessage struct {
	HeaderFields   // Embedded struct
	MetadataFields // Embedded struct containing another embedded struct
	Data           string
}

// Test _ field skipping
type StructWithPadding struct {
	Field1 string
	_      int32  // Should be skipped during encoding/decoding
	Field2 uint64
	_      string // Should be skipped during encoding/decoding
	Field3 bool
}

type EmbeddedWithPadding struct {
	Value1 int
	_      float64 // Should be skipped
	Value2 string
}

type StructWithEmbeddedPadding struct {
	Start string
	EmbeddedWithPadding
	_   int  // Should be skipped
	End bool
}

// Test custom integer types without TextUnmarshaler
type CustomInt32 int32
type CustomUint64 uint64
type CustomFloat64 float64
type CustomBool bool
type CustomString string

type CustomTypesStruct struct {
	IntField    CustomInt32
	UintField   CustomUint64
	FloatField  CustomFloat64
	BoolField   CustomBool
	StringField CustomString
}

// Test [N]byte arrays as strings (for Unicore Char[N] fields)
type ByteArrayStruct struct {
	ShortName  [8]byte  // Short null-terminated string
	LongName   [32]byte // Longer null-terminated string
	NormalField string  // Regular string field
}

func TestCanonical(t *testing.T) {
	// Test cases with canonical string representation - tests both encode and decode
	tests := []struct {
		name   string
		fields []string
		value  interface{}
	}{
		{
			name:   "basic types",
			fields: []string{"hello", "42", "123456789", "3.14159", "true"},
			value: BasicStruct{
				Str:    "hello",
				Int32:  42,
				Uint64: 123456789,
				Float:  3.14159,
				Bool:   true,
			},
		},
		{
			name:   "custom marshal/unmarshal OK",
			fields: []string{"test", "OK", "100"},
			value: CustomStruct{
				Name:   "test",
				Status: StatusOK,
				Value:  100,
			},
		},
		{
			name:   "custom marshal/unmarshal ERROR",
			fields: []string{"test", "ERROR", "999"},
			value: CustomStruct{
				Name:   "test",
				Status: StatusError,
				Value:  999,
			},
		},
		{
			name:   "zero values",
			fields: []string{"", "0", "0", "0", "false"},
			value:  BasicStruct{},
		},
		{
			name:   "negative numbers",
			fields: []string{"", "-42", "0", "-3.14", "false"},
			value: BasicStruct{
				Str:    "",
				Int32:  -42,
				Uint64: 0,
				Float:  -3.14,
				Bool:   false,
			},
		},
		{
			name:   "signed integers max values",
			fields: []string{"9223372036854775807", "127", "32767", "2147483647", "9223372036854775807"},
			value: SignedIntsStruct{
				Int:   9223372036854775807,
				Int8:  127,
				Int16: 32767,
				Int32: 2147483647,
				Int64: 9223372036854775807,
			},
		},
		{
			name:   "signed integers min values",
			fields: []string{"-9223372036854775808", "-128", "-32768", "-2147483648", "-9223372036854775808"},
			value: SignedIntsStruct{
				Int:   -9223372036854775808,
				Int8:  -128,
				Int16: -32768,
				Int32: -2147483648,
				Int64: -9223372036854775808,
			},
		},
		{
			name:   "unsigned integers max values",
			fields: []string{"18446744073709551615", "255", "65535", "4294967295", "18446744073709551615"},
			value: UnsignedIntsStruct{
				Uint:   18446744073709551615,
				Uint8:  255,
				Uint16: 65535,
				Uint32: 4294967295,
				Uint64: 18446744073709551615,
			},
		},
		{
			name:   "embedded structs basic",
			fields: []string{"BESTNAV", "1", "12345", "49.246292", "-123.003617", "46.405", "3735928559"},
			value: CompleteMessage{
				HeaderFields: HeaderFields{
					MessageName: "BESTNAV",
					Port:        1,
					Sequence:    12345,
				},
				PayloadFields: PayloadFields{
					Lat: 49.246292,
					Lon: -123.003617,
					Alt: 46.405,
				},
				Checksum: 3735928559, // 0xDEADBEEF
			},
		},
		{
			name:   "embedded structs zero values",
			fields: []string{"", "0", "0", "0", "0", "0", "0"},
			value:  CompleteMessage{},
		},
		{
			name:   "nested embedded structs",
			fields: []string{"VERSION", "2", "54321", "1234567890", "500", "receiver", "test_data"},
			value: NestedMessage{
				HeaderFields: HeaderFields{
					MessageName: "VERSION",
					Port:        2,
					Sequence:    54321,
				},
				MetadataFields: MetadataFields{
					TimestampFields: TimestampFields{
						Seconds: 1234567890,
						Nanos:   500,
					},
					Source: "receiver",
				},
				Data: "test_data",
			},
		},
		{
			name:   "nested embedded structs zero values",
			fields: []string{"", "0", "0", "0", "0", "", ""},
			value:  NestedMessage{},
		},
		{
			name:   "struct with _ field padding",
			fields: []string{"hello", "12345", "true"}, // Only 3 fields, _ fields are skipped
			value: StructWithPadding{
				Field1: "hello",
				// _ fields are not encoded/decoded
				Field2: 12345,
				// _ fields are not encoded/decoded  
				Field3: true,
			},
		},
		{
			name:   "struct with embedded _ field padding",
			fields: []string{"start", "42", "test", "false"}, // 4 fields, _ field in embedded struct is skipped
			value: StructWithEmbeddedPadding{
				Start: "start",
				EmbeddedWithPadding: EmbeddedWithPadding{
					Value1: 42,
					// _ field skipped
					Value2: "test",
				},
				// _ field skipped
				End: false,
			},
		},
		{
			name:   "custom types without TextUnmarshaler",
			fields: []string{"-123", "456789", "3.14159", "true", "custom_string"},
			value: CustomTypesStruct{
				IntField:    CustomInt32(-123),
				UintField:   CustomUint64(456789),
				FloatField:  CustomFloat64(3.14159),
				BoolField:   CustomBool(true),
				StringField: CustomString("custom_string"),
			},
		},
		{
			name:   "byte arrays as strings",
			fields: []string{"UM980", "R4.10Build13504", "normal"},
			value: func() ByteArrayStruct {
				var s ByteArrayStruct
				// Manually set up expected byte arrays (null-terminated)
				copy(s.ShortName[:], "UM980")    // "UM980" + null bytes
				copy(s.LongName[:], "R4.10Build13504") // "R4.10Build13504" + null bytes
				s.NormalField = "normal"
				return s
			}(),
		},
		{
			name:   "empty struct with no fields",
			fields: []string{},
			value:  struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Encode: value -> fields
			encodedFields, err := Encode(tt.value)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			if !reflect.DeepEqual(encodedFields, tt.fields) {
				t.Errorf("Encode: expected %v, got %v", tt.fields, encodedFields)
			}
			
			// Test Decode: fields -> value
			valueType := reflect.TypeOf(tt.value)
			target := reflect.New(valueType).Interface()
			
			err = Decode(tt.fields, target)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			
			decodedValue := reflect.ValueOf(target).Elem().Interface()
			if !reflect.DeepEqual(decodedValue, tt.value) {
				t.Errorf("Decode: expected %+v, got %+v", tt.value, decodedValue)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	// Test cases for decode-specific scenarios (non-canonical formats)
	tests := []struct {
		name     string
		fields   []string
		expected interface{}
	}{
		{
			name:   "fewer fields than struct",
			fields: []string{"hello", "42"},
			expected: BasicStruct{
				Str:    "hello",
				Int32:  42,
				Uint64: 0,     // zero value
				Float:  0,     // zero value
				Bool:   false, // zero value
			},
		},
		{
			name:   "alternative bool formats",
			fields: []string{"test", "0", "0", "0", "1"},
			expected: BasicStruct{
				Str:    "test",
				Int32:  0,
				Uint64: 0,
				Float:  0,
				Bool:   true, // "1" -> true
			},
		},
		{
			name:   "hex unsigned integers",
			fields: []string{"0xff", "0xff", "0xffff", "0xffffffff", "0xffffffffffffffff"},
			expected: UnsignedIntsStruct{
				Uint:   255,
				Uint8:  255,
				Uint16: 65535,
				Uint32: 4294967295,
				Uint64: 18446744073709551615,
			},
		},
		{
			name:   "mixed decimal and hex",
			fields: []string{"255", "0xff", "65535", "0xffffffff", "18446744073709551615"},
			expected: UnsignedIntsStruct{
				Uint:   255,
				Uint8:  255,
				Uint16: 65535,
				Uint32: 4294967295,
				Uint64: 18446744073709551615,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create zero value for decoding
			expectedType := reflect.TypeOf(tt.expected)
			target := reflect.New(expectedType).Interface()
			
			err := Decode(tt.fields, target)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			
			result := reflect.ValueOf(target).Elem().Interface()
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		target interface{}
	}{
		{
			name:   "non-pointer",
			fields: []string{"test"},
			target: BasicStruct{},
		},
		{
			name:   "invalid number",
			fields: []string{"not-a-number"},
			target: &IntStruct{},
		},
		{
			name:   "custom type error",
			fields: []string{"INVALID"},
			target: &struct{ Status Status }{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Decode(tt.fields, tt.target)
			if err == nil {
				t.Error("expected error but got none")
			}
		})
	}
}