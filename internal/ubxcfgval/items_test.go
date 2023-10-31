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
