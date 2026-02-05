package opt

import (
	"encoding/json"
	"reflect"
	"testing"
)

type testStruct struct {
	Name    string       `json:"name"`
	Age     Val[int]     `json:"age,omitzero"`
	Score   Val[float64] `json:"score,omitzero"`
	Active  Val[bool]    `json:"active,omitzero"`
	Comment Val[string]  `json:"comment,omitzero"`
}

func TestJSONMarshal(t *testing.T) {
	tests := []struct {
		name string
		in   testStruct
		want string
	}{
		{
			name: "all unset",
			in:   testStruct{Name: "test"},
			want: `{"name":"test"}`,
		},
		{
			name: "all set",
			in: testStruct{
				Name:    "test",
				Age:     Make(42),
				Score:   Make(3.14),
				Active:  Make(true),
				Comment: Make("hello"),
			},
			want: `{"name":"test","age":42,"score":3.14,"active":true,"comment":"hello"}`,
		},
		{
			name: "partial set",
			in: testStruct{
				Name:  "test",
				Age:   Make(25),
				Score: Make(0.0), // zero value but set
			},
			want: `{"name":"test","age":25,"score":0}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.in)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestJSONUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want testStruct
	}{
		{
			name: "all present",
			in:   `{"name":"test","age":42,"score":3.14}`,
			want: testStruct{Name: "test", Age: Make(42), Score: Make(3.14)},
		},
		{
			name: "missing fields",
			in:   `{"name":"test"}`,
			want: testStruct{Name: "test"},
		},
		{
			name: "partial",
			in:   `{"name":"test","age":42}`,
			want: testStruct{Name: "test", Age: Make(42)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got testStruct
			if err := json.Unmarshal([]byte(tt.in), &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestJSONUnmarshalNull(t *testing.T) {
	var got testStruct
	err := json.Unmarshal([]byte(`{"name":"test","age":null}`), &got)
	if err == nil {
		t.Error("expected error for null value")
	}
}

func TestTextUnmarshal(t *testing.T) {
	t.Run("int empty", func(t *testing.T) {
		var v Val[int]
		if err := v.UnmarshalText([]byte("")); err != nil {
			t.Fatal(err)
		}
		if v.IsSet() {
			t.Error("should not be set")
		}
	})
	t.Run("int value", func(t *testing.T) {
		var v Val[int]
		if err := v.UnmarshalText([]byte("42")); err != nil {
			t.Fatal(err)
		}
		if !v.IsSet() || v.Get() != 42 {
			t.Errorf("got set=%v val=%v, want set=true val=42", v.IsSet(), v.Get())
		}
	})
	t.Run("uint hex", func(t *testing.T) {
		var v Val[uint]
		if err := v.UnmarshalText([]byte("0x2a")); err != nil {
			t.Fatal(err)
		}
		if !v.IsSet() || v.Get() != 42 {
			t.Errorf("got set=%v val=%v, want set=true val=42", v.IsSet(), v.Get())
		}
	})
}

func TestParseText(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		var v int
		if err := ParseText("42", &v); err != nil {
			t.Fatal(err)
		}
		if v != 42 {
			t.Errorf("got %d, want 42", v)
		}
	})
	t.Run("float64", func(t *testing.T) {
		var v float64
		if err := ParseText("3.14", &v); err != nil {
			t.Fatal(err)
		}
		if v != 3.14 {
			t.Errorf("got %f, want 3.14", v)
		}
	})
	t.Run("bool", func(t *testing.T) {
		var v bool
		if err := ParseText("true", &v); err != nil {
			t.Fatal(err)
		}
		if !v {
			t.Error("got false, want true")
		}
	})
	t.Run("string", func(t *testing.T) {
		var v string
		if err := ParseText("hello", &v); err != nil {
			t.Fatal(err)
		}
		if v != "hello" {
			t.Errorf("got %q, want %q", v, "hello")
		}
	})
	t.Run("uint hex", func(t *testing.T) {
		var v uint
		if err := ParseText("0xff", &v); err != nil {
			t.Fatal(err)
		}
		if v != 255 {
			t.Errorf("got %d, want 255", v)
		}
	})
}

func TestIsZero(t *testing.T) {
	var unset Val[int]
	if !unset.IsZero() {
		t.Error("unset should be zero")
	}
	set := Make(0)
	if set.IsZero() {
		t.Error("set to zero value should not be IsZero")
	}
}

func TestTextRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		val  any
	}{
		{"int zero", int(0)},
		{"int positive", int(42)},
		{"int negative", int(-123)},
		{"int max", int(9223372036854775807)},
		{"int8", int8(-128)},
		{"int16", int16(32767)},
		{"int32", int32(-2147483648)},
		{"int64", int64(9223372036854775807)},
		{"uint zero", uint(0)},
		{"uint", uint(42)},
		{"uint8", uint8(255)},
		{"uint16", uint16(65535)},
		{"uint32", uint32(4294967295)},
		{"uint64", uint64(18446744073709551615)},
		{"float32 zero", float32(0)},
		{"float32 positive", float32(3.14)},
		{"float32 negative", float32(-2.5)},
		{"float32 small", float32(1e-10)},
		{"float32 large", float32(1e30)},
		{"float64 zero", float64(0)},
		{"float64 positive", float64(3.141592653589793)},
		{"float64 negative", float64(-2.718281828459045)},
		{"float64 small", float64(1e-100)},
		{"float64 large", float64(1e100)},
		{"bool true", true},
		{"bool false", false},
		{"string empty", ""},
		{"string", "hello world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := FormatText(tt.val)
			if err != nil {
				t.Fatalf("FormatText error: %v", err)
			}
			switch v := tt.val.(type) {
			case int:
				var got int
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case int8:
				var got int8
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case int16:
				var got int16
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case int32:
				var got int32
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case int64:
				var got int64
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case uint:
				var got uint
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case uint8:
				var got uint8
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case uint16:
				var got uint16
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case uint32:
				var got uint32
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case uint64:
				var got uint64
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case float32:
				var got float32
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case float64:
				var got float64
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case bool:
				var got bool
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %v, want %v (text: %q)", got, v, text)
				}
			case string:
				var got string
				if err := ParseText(string(text), &got); err != nil {
					t.Fatalf("ParseText error: %v", err)
				}
				if got != v {
					t.Errorf("got %q, want %q (text: %q)", got, v, text)
				}
			}
		})
	}
}
