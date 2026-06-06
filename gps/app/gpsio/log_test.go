package gpsio

import (
	"encoding/json"
	"testing"
)

func TestHexStringUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
		want HexString
		err  bool
	}{
		{name: "plain hex", data: `"b5620107"`, want: HexString{0xb5, 0x62, 0x01, 0x07}},
		{name: "upper case hex", data: `"B5620107"`, want: HexString{0xb5, 0x62, 0x01, 0x07}},
		{name: "escaped hex", data: `"b5\u0036\u0032"`, want: HexString{0xb5, 0x62}},
		{name: "null", data: `null`, want: nil},
		{name: "odd length", data: `"b56"`, err: true},
		{name: "invalid hex", data: `"xx"`, err: true},
		{name: "invalid json", data: `b562`, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got HexString
			err := json.Unmarshal([]byte(tt.data), &got)
			if tt.err {
				if err == nil {
					t.Fatal("Unmarshal succeeded, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if string(got) != string(tt.want) {
				t.Fatalf("HexString = %x, want %x", []byte(got), []byte(tt.want))
			}
		})
	}
}
