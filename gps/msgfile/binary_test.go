package msgfile

import (
	"testing"
	"time"
)

func TestLoadBinaryMsgSimple(t *testing.T) {
	toml := `[[binary]]
hex = "B562068A0900"

[[binary]]
hex = "DEADBEEF"
`
	mf := loadFromString(t, toml)
	if len(mf.Binary) != 2 {
		t.Fatalf("expected 2 binary messages, got %d", len(mf.Binary))
	}
	if mf.Binary[0].Hex != "B562068A0900" {
		t.Errorf("expected first hex, got %q", mf.Binary[0].Hex)
	}
	if mf.Binary[1].Hex != "DEADBEEF" {
		t.Errorf("expected second hex, got %q", mf.Binary[1].Hex)
	}
}

func TestBinaryMsgsToRaw(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []RawMsg
	}{
		{
			name: "simple binary",
			toml: `[[binary]]
hex = "DEADBEEF"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, Delay: 0, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "hex with whitespace",
			toml: `[[binary]]
hex = "DE AD BE EF"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF}, Delay: 0, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "with delay",
			toml: `[[binary]]
hex = "B562"
delay = 0.5
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte{0xB5, 0x62}, Delay: 500 * time.Millisecond, Tag: "", Index: 0, Count: 1},
			},
		},
		{
			name: "default delay",
			toml: `[default.binary]
delay = 0.1

[[binary]]
hex = "AA"

[[binary]]
hex = "BB"
`,
			tags: []string{""},
			expected: []RawMsg{
				{Bytes: []byte{0xAA}, Delay: 100 * time.Millisecond, Tag: "", Index: 0, Count: 2},
				{Bytes: []byte{0xBB}, Delay: 100 * time.Millisecond, Tag: "", Index: 1, Count: 2},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			raw, err := ToRaw(msgs, false)
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if !rawMsgsEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}

func TestValidateBinary(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		tags    []string
		wantErr bool
	}{
		{
			name: "valid binary",
			toml: `[[binary]]
hex = "DEADBEEF"
`,
			tags:    []string{""},
			wantErr: false,
		},
		{
			name: "empty hex",
			toml: `[[binary]]
hex = ""
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "invalid hex chars",
			toml: `[[binary]]
hex = "GHIJ"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "odd length hex",
			toml: `[[binary]]
hex = "ABC"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "default hex not empty",
			toml: `[default.binary]
hex = "AA"

[[binary]]
hex = "BB"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "negative delay in binary",
			toml: `[[binary]]
hex = "AA"
delay = -1.0
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "negative delay in default binary",
			toml: `[default.binary]
delay = -0.5

[[binary]]
hex = "AA"
`,
			tags:    []string{""},
			wantErr: true,
		},
		{
			name: "mixed line and binary in file",
			toml: `[[line]]
text = "TEST"
tag = "lines"

[[binary]]
hex = "AA"
tag = "bins"
`,
			tags:    []string{"lines"},
			wantErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			err := validateMsgs(mf, tc.tags)
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTaggedBinaryMsgs(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		tags     []string
		expected []RawMsg
	}{
		{
			name: "filter by tag",
			toml: `[[binary]]
hex = "AA"
tag = "setup"

[[binary]]
hex = "BB"
tag = "other"
`,
			tags: []string{"setup"},
			expected: []RawMsg{
				{Bytes: []byte{0xAA}, Delay: 0, Tag: "setup", Index: 0, Count: 1},
			},
		},
		{
			name: "default tag",
			toml: `[default.binary]
tag = "init"

[[binary]]
hex = "AA"

[[binary]]
hex = "BB"
tag = "other"
`,
			tags: []string{"init"},
			expected: []RawMsg{
				{Bytes: []byte{0xAA}, Delay: 0, Tag: "init", Index: 0, Count: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs error: %v", err)
			}
			raw, err := ToRaw(msgs, false)
			if err != nil {
				t.Fatalf("ToRaw error: %v", err)
			}
			if !rawMsgsEqual(raw, tc.expected) {
				t.Errorf("got %+v, expected %+v", raw, tc.expected)
			}
		})
	}
}
