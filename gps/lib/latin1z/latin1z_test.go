package latin1z

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestToString(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect string
	}{
		{
			name:   "ascii terminated",
			input:  []byte{'h', 'i', 0, 'x', 'x'},
			expect: "hi",
		},
		{
			name:   "no nul",
			input:  []byte{'a', 'b', 'c'},
			expect: "abc",
		},
		{
			name:   "leading nul",
			input:  []byte{0, 'a', 'b'},
			expect: "",
		},
		{
			name:   "latin-1 high bytes",
			input:  []byte{'c', 'a', 'f', 0xe9, 0},
			expect: "caf\u00e9",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ToString(tc.input)
			if got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestStringZMarshalJSON(t *testing.T) {
	v := StringZ5{'c', 'a', 'f', 0xe9, 0}
	got, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	expect := []byte(`"café"`)
	if !reflect.DeepEqual(got, expect) {
		t.Errorf("got %s, want %s", got, expect)
	}
}
