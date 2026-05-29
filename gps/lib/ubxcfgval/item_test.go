package ubxcfgval

import "testing"

func TestItems(t *testing.T) {
	valgets, err := parseUCenterConfig(f9pCfgTxt)
	if err != nil || len(valgets) == 0 {
		t.Fatalf("could not parse config text")
	}
	keysCount := 0
	for i, msg := range valgets {
		// U-center format doesn't include the two sync bytes not the two-byte checksum.
		// So to get the config data, we just have to skip cls+id, 2-byte length, and 4 byte fixed part.
		cfgData := msg[8:]
		var m Map
		err := m.UnmarshalBinary(cfgData)
		if err != nil {
			t.Errorf("test %d: could not unmarshal: %v", i, err)
		} else {
			keysCount += len(m)
		}
		data, err := m.MarshalBinary()
		if err != nil {
			t.Errorf("test %d: could not marshal %v", i, err)
		}
		if len(data) != len(cfgData) {
			t.Errorf("test %d: marshaled data length %d, expected %d", i, len(data), len(cfgData))
		}
		if string(data) != string(cfgData) {
			t.Errorf("test %d: marshaled data does not match original", i)
		}
	}
	if keysCount != 1138 {
		t.Errorf("found %d config items, expected at least 1138", keysCount)
	}
}

func TestValueBits(t *testing.T) {
	if valueBits(0x10260013) != 1 {
		t.Error("expected valueBits(0x10260013) == 1")
	}
	if valueBits(0x30260015) != 16 {
		t.Error("expected valueBits(0x30260015) == 16")
	}
	if valueBits(0x50110063) != 64 {
		t.Error("expected valueBits(0x50110063) == 64")
	}
}

func valueBits(k uint32) int {
	return Key(k).valueBits()
}

func TestValueBytes(t *testing.T) {
	if valueBytes(0x20240011) != 1 || valueBytes(0x10240012) != 1 || valueBytes(0x40240021) != 4 || valueBytes(0x50110062) != 8 || valueBytes(0x301100b5) != 2 {
		t.Error("valueBytes failed")
	}
}

func valueBytes(k uint32) int {
	return Key(k).NValueBytes()
}
