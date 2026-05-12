package msgfile

import (
	"bytes"
	"strings"
	"testing"
)

// rawValPayload for a single CFG-VALSET item, used as the "old" raw form
// equivalent the new [[ubxval]] message should encode to.
func rawValPayload(layer byte, key uint32, value byte) []byte {
	return []byte{
		0,     // version (no transaction)
		layer, // layers
		0,     // transaction
		0,     // reserved
		byte(key), byte(key >> 8), byte(key >> 16), byte(key >> 24),
		value,
	}
}

func ubxPacketPayload(t *testing.T, raw []byte) []byte {
	t.Helper()
	if len(raw) < 8 {
		t.Fatalf("packet too short: %d", len(raw))
	}
	if raw[0] != 0xB5 || raw[1] != 0x62 {
		t.Fatalf("bad sync: %02x %02x", raw[0], raw[1])
	}
	payloadLen := int(raw[4]) | int(raw[5])<<8
	if len(raw) != 8+payloadLen {
		t.Fatalf("packet length mismatch: header says %d, packet has %d", payloadLen, len(raw)-8)
	}
	return raw[6 : 6+payloadLen]
}

func loadAndRaw(t *testing.T, toml string, tags []string, save bool) []RawMsg {
	t.Helper()
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs(tags)
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	raw, err := ToRaw(msgs, "", save)
	if err != nil {
		t.Fatalf("ToRaw: %v", err)
	}
	return raw
}

func TestUBXValRamMatchesRaw(t *testing.T) {
	toml := `[[ubxval]]
tag = "osnma-on"
keys = [0x10350005]
types = "U1"
values = [1]
`
	raw := loadAndRaw(t, toml, []string{"osnma-on"}, false)
	if len(raw) != 1 {
		t.Fatalf("expected 1 packet, got %d", len(raw))
	}
	got := ubxPacketPayload(t, raw[0].Bytes)
	want := rawValPayload(0x01, 0x10350005, 1)
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch:\n got %x\nwant %x", got, want)
	}
	// Sanity: class/id are 0x06/0x8A.
	if raw[0].Bytes[2] != 0x06 || raw[0].Bytes[3] != 0x8A {
		t.Errorf("class/id: got %02x %02x", raw[0].Bytes[2], raw[0].Bytes[3])
	}
}

func TestUBXValSaveLayers(t *testing.T) {
	toml := `[[ubxval]]
tag = "osnma-on"
keys = [0x10350005]
types = "U1"
values = [1]
`
	raw := loadAndRaw(t, toml, []string{"osnma-on"}, true)
	got := ubxPacketPayload(t, raw[0].Bytes)
	want := rawValPayload(0x07, 0x10350005, 1) // RAM|BBR|Flash
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestUBXValMultipleItemsInOnePacket(t *testing.T) {
	toml := `[[ubxval]]
tag = "osnma-and-only-auth"
keys = [0x10350005, 0x101100DD]
types = "U1U1"
values = [1, 1]
`
	raw := loadAndRaw(t, toml, []string{"osnma-and-only-auth"}, false)
	if len(raw) != 1 {
		t.Fatalf("expected 1 packet, got %d", len(raw))
	}
	got := ubxPacketPayload(t, raw[0].Bytes)
	want := []byte{
		0, 0x01, 0, 0,
		0x05, 0x00, 0x35, 0x10, 0x01,
		0xDD, 0x00, 0x11, 0x10, 0x01,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestUBXValCountMismatch(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{
			name: "missing value",
			toml: `[[ubxval]]
tag = "bad"
keys = [0x10350005, 0x101100DD]
types = "U1U1"
values = [1]
`,
		},
		{
			name: "extra value",
			toml: `[[ubxval]]
tag = "bad"
keys = [0x10350005]
types = "U1"
values = [1, 1]
`,
		},
		{
			name: "missing type",
			toml: `[[ubxval]]
tag = "bad"
keys = [0x10350005, 0x101100DD]
types = "U1"
values = [1, 1]
`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs([]string{"bad"})
			if err != nil {
				t.Fatalf("TaggedMsgs: %v", err)
			}
			if _, err := ToRaw(msgs, "", false); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestUBXValHexAndU4Encoding(t *testing.T) {
	toml := `[[ubxval]]
tag = "u4-key"
keys = [0x40050002]
types = "U4"
values = [0xDEADBEEF]
`
	raw := loadAndRaw(t, toml, []string{"u4-key"}, false)
	got := ubxPacketPayload(t, raw[0].Bytes)
	want := []byte{
		0, 0x01, 0, 0,
		0x02, 0x00, 0x05, 0x40,
		0xEF, 0xBE, 0xAD, 0xDE,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestUBXValTypeWidthMismatch(t *testing.T) {
	// 0x10350005 is an L (1-byte) key. Using U2 widens to 2 bytes -> error.
	toml := `[[ubxval]]
tag = "bad"
keys = [0x10350005]
types = "U2"
values = [1]
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"bad"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	if _, err := ToRaw(msgs, "", false); err == nil {
		t.Fatal("expected width mismatch error, got nil")
	}
}

func TestUBXValInvalidKeyValueSize(t *testing.T) {
	// 0x10000005 has size nibble 0 in bits 28..31 -> no recognized value size.
	toml := `[[ubxval]]
tag = "bad"
keys = [0x00000005]
types = "U1"
values = [1]
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"bad"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	if _, err := ToRaw(msgs, "", false); err == nil {
		t.Fatal("expected invalid key error, got nil")
	}
}

func TestUBXValRejects8ByteIntegerType(t *testing.T) {
	// U8/I8 are not in the grammar; parseTypes should fail with unknown specifier.
	toml := `[[ubxval]]
tag = "bad"
keys = [0x50050002]
types = "U8"
values = [1]
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"bad"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	if _, err := ToRaw(msgs, "", false); err == nil {
		t.Fatal("expected error for 8-byte integer type, got nil")
	}
}

func TestUBXValU1RangeChecks(t *testing.T) {
	toml := `[[ubxval]]
tag = "bad"
keys = [0x10350005]
types = "U1"
values = [256]
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"bad"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	if _, err := ToRaw(msgs, "", false); err == nil {
		t.Fatal("expected U1 range error, got nil")
	}
}

func TestUBXValFloatNumericSemantics(t *testing.T) {
	// CFG-NAVHPG-DGNSSMODE is U1; for R4 we need a different key. Construct
	// a fake R4 key id: bits 28..31 = 0x4 = R4 width; bits 16..23 = group 5
	// = "TP" group; low bits = 0x0010. The actual key id 0x40050010 is
	// CFG-RATE-MEAS in u-blox; that is U4 width. Use 0x40050010 with U4
	// for the integer-vs-float coverage, and a synthetic R4 key 0x40-pattern
	// where width=4 matches.
	//
	// Use a constructed R4 key: 0x40050010 with R4 type. The encoded bytes
	// for value=1.5 must be 0x00 0x00 0xC0 0x3F (little-endian float32).
	toml := `[[ubxval]]
tag = "r4-key"
keys = [0x40050010]
types = "R4"
values = [1.5]
`
	raw := loadAndRaw(t, toml, []string{"r4-key"}, false)
	got := ubxPacketPayload(t, raw[0].Bytes)
	want := []byte{
		0, 0x01, 0, 0,
		0x10, 0x00, 0x05, 0x40,
		0x00, 0x00, 0xC0, 0x3F,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestToRawSaveOnNonUBXValFails(t *testing.T) {
	tests := []struct {
		name string
		toml string
		tags []string
	}{
		{
			name: "line",
			toml: `[[line]]
text = "HELLO"
tag = "t"
`,
			tags: []string{"t"},
		},
		{
			name: "nmea",
			toml: `[[nmea]]
text = "PQTMVERNO"
tag = "t"
`,
			tags: []string{"t"},
		},
		{
			name: "raw ubx",
			toml: `[[ubx]]
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x10350005, 1]
tag = "t"
`,
			tags: []string{"t"},
		},
		{
			name: "binary",
			toml: `[[binary]]
hex = "AA"
tag = "t"
`,
			tags: []string{"t"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := loadFromString(t, tc.toml)
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatalf("TaggedMsgs: %v", err)
			}
			_, err = ToRaw(msgs, "", true)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "ubxval") {
				t.Errorf("error %q does not mention ubxval", err)
			}
		})
	}
}

func TestToRawNoSaveKeepsExistingTypesWorking(t *testing.T) {
	toml := `[[line]]
text = "HELLO"
tag = "t"
`
	raw := loadAndRaw(t, toml, []string{"t"}, false)
	if len(raw) != 1 {
		t.Fatalf("expected 1 message, got %d", len(raw))
	}
}

// TestUBXValMatchesOldRawUBX checks that for the same key/value, the
// emitted CFG-VALSET payload from [[ubxval]] without --save is byte-for-byte
// the same as the old raw [[ubx]] CFG-VALSET form.
func TestUBXValMatchesOldRawUBX(t *testing.T) {
	rawToml := `[[ubx]]
tag = "old"
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x10350005, 1]
`
	newToml := `[[ubxval]]
tag = "new"
keys = [0x10350005]
types = "U1"
values = [1]
`
	oldRaw := loadAndRaw(t, rawToml, []string{"old"}, false)
	newRaw := loadAndRaw(t, newToml, []string{"new"}, false)
	if !bytes.Equal(oldRaw[0].Bytes, newRaw[0].Bytes) {
		t.Errorf("packets differ:\n old %x\n new %x", oldRaw[0].Bytes, newRaw[0].Bytes)
	}
}

func TestUBXValLoadFromUbx9TOML(t *testing.T) {
	// Mirrors the intermediate migration: a CFG-MSGOUT_*_USB key set via
	// [[ubxval]]. Verifies the key byte width is respected and that
	// the emitted payload matches the equivalent raw [[ubx]] write.
	rawToml := `[[ubx]]
tag = "old"
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x2091004A, 1]
`
	newToml := `[[ubxval]]
tag = "new"
keys = [0x2091004A]
types = "U1"
values = [1]
`
	oldRaw := loadAndRaw(t, rawToml, []string{"old"}, false)
	newRaw := loadAndRaw(t, newToml, []string{"new"}, false)
	if !bytes.Equal(oldRaw[0].Bytes, newRaw[0].Bytes) {
		t.Errorf("ubx9-style migration packets differ:\n old %x\n new %x", oldRaw[0].Bytes, newRaw[0].Bytes)
	}
}
