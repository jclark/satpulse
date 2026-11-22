package msgfile

import (
	"bytes"
	"strings"
	"testing"
)

func loadAndRawPort(t *testing.T, toml string, tags []string, port string, save bool) []RawMsg {
	t.Helper()
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs(tags)
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	raw, err := ToRaw(msgs, port, save)
	if err != nil {
		t.Fatalf("ToRaw: %v", err)
	}
	return raw
}

func TestUBXValPortEncodingPerPort(t *testing.T) {
	// CFG-MSGOUT-NAV_TIMEUTC: base 0x2091005B; i2c=5B uart1=5C uart2=5D usb=5E spi=5F.
	toml := `[[ubxvalport]]
tag = "t"
key.usb = 0x2091005E
value = 1
`
	cases := []struct {
		port    string
		wantKey uint32
	}{
		{"i2c", 0x2091005B},
		{"uart1", 0x2091005C},
		{"uart2", 0x2091005D},
		{"usb", 0x2091005E},
		{"spi", 0x2091005F},
	}
	for _, tc := range cases {
		t.Run(tc.port, func(t *testing.T) {
			raw := loadAndRawPort(t, toml, []string{"t"}, tc.port, false)
			if len(raw) != 1 {
				t.Fatalf("expected 1 packet, got %d", len(raw))
			}
			got := ubxPacketPayload(t, raw[0].Bytes)
			want := rawValPayload(0x01, tc.wantKey, 1)
			if !bytes.Equal(got, want) {
				t.Errorf("payload mismatch for %s:\n got %x\nwant %x", tc.port, got, want)
			}
			if raw[0].Bytes[2] != 0x06 || raw[0].Bytes[3] != 0x8A {
				t.Errorf("class/id: got %02x %02x", raw[0].Bytes[2], raw[0].Bytes[3])
			}
		})
	}
}

func TestUBXValPortPortCaseInsensitive(t *testing.T) {
	toml := `[[ubxvalport]]
tag = "t"
key.usb = 0x2091005E
value = 1
`
	for _, port := range []string{"USB", "Usb", "uSb"} {
		raw := loadAndRawPort(t, toml, []string{"t"}, port, false)
		got := ubxPacketPayload(t, raw[0].Bytes)
		want := rawValPayload(0x01, 0x2091005E, 1)
		if !bytes.Equal(got, want) {
			t.Errorf("port %q: payload mismatch:\n got %x\nwant %x", port, got, want)
		}
	}
}

func TestUBXValPortRamLayer(t *testing.T) {
	toml := `[[ubxvalport]]
tag = "t"
key.usb = 0x2091005E
value = 1
`
	raw := loadAndRawPort(t, toml, []string{"t"}, "usb", false)
	got := ubxPacketPayload(t, raw[0].Bytes)
	want := rawValPayload(0x01, 0x2091005E, 1)
	if !bytes.Equal(got, want) {
		t.Errorf("RAM layer payload mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestUBXValPortSaveLayer(t *testing.T) {
	toml := `[[ubxvalport]]
tag = "t"
key.usb = 0x2091005E
value = 1
`
	raw := loadAndRawPort(t, toml, []string{"t"}, "usb", true)
	got := ubxPacketPayload(t, raw[0].Bytes)
	want := rawValPayload(0x07, 0x2091005E, 1) // RAM|BBR|Flash
	if !bytes.Equal(got, want) {
		t.Errorf("save layer payload mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestUBXValPortInferFromAnyPort(t *testing.T) {
	// CFG-MSGOUT-NAV_TIMEUTC base 0x2091005B. Supply any single port and ask
	// for any other; inference should pick the right key.
	cases := []struct {
		suppliedPort string
		suppliedKey  uint32
		targetPort   string
		wantKey      uint32
	}{
		{"i2c", 0x2091005B, "usb", 0x2091005E},
		{"uart1", 0x2091005C, "spi", 0x2091005F},
		{"uart2", 0x2091005D, "i2c", 0x2091005B},
		{"usb", 0x2091005E, "uart1", 0x2091005C},
		{"spi", 0x2091005F, "uart2", 0x2091005D},
	}
	for _, tc := range cases {
		t.Run(tc.suppliedPort+"->"+tc.targetPort, func(t *testing.T) {
			toml := "[[ubxvalport]]\ntag = \"t\"\nkey." + tc.suppliedPort + " = " +
				formatHex(tc.suppliedKey) + "\nvalue = 1\n"
			raw := loadAndRawPort(t, toml, []string{"t"}, tc.targetPort, false)
			got := ubxPacketPayload(t, raw[0].Bytes)
			want := rawValPayload(0x01, tc.wantKey, 1)
			if !bytes.Equal(got, want) {
				t.Errorf("payload mismatch:\n got %x\nwant %x", got, want)
			}
		})
	}
}

func TestUBXValPortInferInfmsgFamily(t *testing.T) {
	// CFG-INFMSG-UBX_USB at 0x20920004. Other ports differ by the standard offset.
	toml := `[[ubxvalport]]
tag = "t"
key.usb = 0x20920004
value = 0x07
`
	raw := loadAndRawPort(t, toml, []string{"t"}, "uart1", false)
	got := ubxPacketPayload(t, raw[0].Bytes)
	// uart1 = base + 1 = 0x20920001 + 1 = 0x20920002.
	want := rawValPayload(0x01, 0x20920002, 0x07)
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch:\n got %x\nwant %x", got, want)
	}
}

func TestUBXValPortInconsistentBaseRejected(t *testing.T) {
	toml := `[[ubxvalport]]
tag = "t"
key.usb   = 0x2091005E
key.uart1 = 0x2091005B
value = 1
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"t"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	if _, err := ToRaw(msgs, "usb", false); err == nil {
		t.Fatal("expected inconsistent-base error, got nil")
	}
}

func TestUBXValPortNonInferableRejectedWithPartialKeys(t *testing.T) {
	// CFG-USBOUTPROT-UBX: 0x10780001. Not in an inferable family.
	toml := `[[ubxvalport]]
tag = "t"
key.usb = 0x10780001
value = 1
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"t"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	if _, err := ToRaw(msgs, "usb", false); err == nil {
		t.Fatal("expected non-inferable error, got nil")
	}
}

func TestUBXValPortFullTableNoOffsetPatternRequired(t *testing.T) {
	// CFG-*OUTPROT-UBX keys do not follow the standard offset pattern.
	// Supplying all five lets the helper bypass the pattern check.
	toml := `[[ubxvalport]]
tag = "t"
key.i2c   = 0x10720001
key.uart1 = 0x10740001
key.uart2 = 0x10760001
key.usb   = 0x10780001
key.spi   = 0x107a0001
value = 1
`
	cases := []struct {
		port    string
		wantKey uint32
	}{
		{"i2c", 0x10720001},
		{"uart1", 0x10740001},
		{"uart2", 0x10760001},
		{"usb", 0x10780001},
		{"spi", 0x107a0001},
	}
	for _, tc := range cases {
		t.Run(tc.port, func(t *testing.T) {
			raw := loadAndRawPort(t, toml, []string{"t"}, tc.port, false)
			got := ubxPacketPayload(t, raw[0].Bytes)
			want := rawValPayload(0x01, tc.wantKey, 1)
			if !bytes.Equal(got, want) {
				t.Errorf("payload mismatch for %s:\n got %x\nwant %x", tc.port, got, want)
			}
		})
	}
}

func TestUBXValPortMissingPortFails(t *testing.T) {
	toml := `[[ubxvalport]]
tag = "t"
key.usb = 0x2091005E
value = 1
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"t"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	_, err = ToRaw(msgs, "", false)
	if err == nil {
		t.Fatal("expected error for missing port, got nil")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("error %q does not mention port", err)
	}
}

func TestUBXValPortInvalidKeyValueSize(t *testing.T) {
	// Key 0x00000005 has size nibble 0 -> no recognized value size.
	// Force all-five so we bypass family check but still hit downstream size check.
	toml := `[[ubxvalport]]
tag = "t"
key.i2c   = 0x00000005
key.uart1 = 0x00000005
key.uart2 = 0x00000005
key.usb   = 0x00000005
key.spi   = 0x00000005
value = 1
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"t"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	if _, err := ToRaw(msgs, "usb", false); err == nil {
		t.Fatal("expected invalid-key error, got nil")
	}
}

func TestUBXValPortSaveAllowed(t *testing.T) {
	toml := `[[ubxvalport]]
tag = "t"
key.usb = 0x2091005E
value = 1
`
	mf := loadFromString(t, toml)
	msgs, err := mf.TaggedMsgs([]string{"t"})
	if err != nil {
		t.Fatalf("TaggedMsgs: %v", err)
	}
	if _, err := ToRaw(msgs, "usb", true); err != nil {
		t.Fatalf("expected --save to be accepted with ubxvalport, got %v", err)
	}
}

func TestUBXValPortMixedTagsRejected(t *testing.T) {
	// Same tag on ubxvalport and a different message type.
	toml := `[[ubxvalport]]
tag = "mixed"
key.usb = 0x2091005E
value = 1

[[line]]
tag = "mixed"
text = "X"
`
	mf := loadFromString(t, toml)
	_, err := mf.TaggedMsgs([]string{"mixed"})
	if err == nil {
		t.Fatal("expected mixed-type error, got nil")
	}
}

func TestUBXValPortMatchesExpectedHexBytes(t *testing.T) {
	// Sanity: a known osnma-monitor entry from the example in the plan.
	toml := `[[ubxvalport]]
tag = "osnma-monitor"
description = "Enable UBX-NAV-TIMETRUSTED"
key.usb = 0x209103AB
value = 1
`
	raw := loadAndRawPort(t, toml, []string{"osnma-monitor"}, "usb", false)
	got := ubxPacketPayload(t, raw[0].Bytes)
	want := rawValPayload(0x01, 0x209103AB, 1)
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch:\n got %x\nwant %x", got, want)
	}
}

func formatHex(v uint32) string {
	const hex = "0123456789ABCDEF"
	buf := []byte{'0', 'x', 0, 0, 0, 0, 0, 0, 0, 0}
	for i := range 8 {
		buf[2+i] = hex[(v>>uint(28-4*i))&0xf]
	}
	return string(buf)
}
