package sbfbin

import "testing"

func TestMeasEpochSignalAccessors(t *testing.T) {
	t1 := MeasEpochChannelType1{Type: MeasType(2<<5) | MeasType(MeasSigIdxExtension), ObsInfo: 6 << 3}
	if got := t1.AntennaID(); got != 2 {
		t.Errorf("Type1 AntennaID = %d, want 2", got)
	}
	if got := t1.SignalNumber(); got != 38 {
		t.Errorf("Type1 SignalNumber = %d, want 38", got)
	}
	t2 := MeasEpochChannelType2{Type: MeasType(3<<5) | MeasType(SigNumGPSL5)}
	if got := t2.AntennaID(); got != 3 {
		t.Errorf("Type2 AntennaID = %d, want 3", got)
	}
	if got := t2.SignalNumber(); got != SigNumGPSL5 {
		t.Errorf("Type2 SignalNumber = %d, want %d", got, SigNumGPSL5)
	}
}
