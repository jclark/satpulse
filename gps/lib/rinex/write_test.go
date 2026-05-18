package rinex

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestWriteObservationFile(t *testing.T) {
	obs := []SignalObservation{
		{
			T:   mustTime(t, "2025-12-17T08:14:06.0080000"),
			Sat: "G07",
			Sig: "1C",
			PR:  opt.Make(23956830.530),
			CP:  opt.Make(125893980.172),
			Do:  opt.Make(2059.717),
			CN0: opt.Make(float32(34)),
			LLI: opt.Make(uint8(1)),
		},
		{
			T:   mustTime(t, "2025-12-17T08:14:06.0080000"),
			Sat: "R06",
			Sig: "1C",
			Frq: opt.Make(int8(-4)),
			PR:  opt.Make(24968868.310),
			CP:  opt.Make(133238671.287),
			Do:  opt.Make(-2790.338),
			CN0: opt.Make(float32(32)),
		},
	}
	var b bytes.Buffer
	opts := WriterOptions{Program: "test", Date: time.Date(2026, time.May, 18, 9, 0, 0, 0, time.UTC)}
	if err := WriteObservationFile(&b, Metadata{}, obs, opts); err != nil {
		t.Fatalf("WriteObservationFile: %v", err)
	}
	s := b.String()
	checks := []string{
		"     3.04           OBSERVATION DATA    M: Mixed            RINEX VERSION / TYPE",
		"G    4 C1C L1C D1C S1C                                      SYS / # / OBS TYPES",
		"R    4 C1C L1C D1C S1C                                      SYS / # / OBS TYPES",
		"  2025    12    17    08    14   06.0080000     GPS         TIME OF FIRST OBS",
		"  1 R06 -4                                                  GLONASS SLOT / FRQ #",
		"> 2025 12 17 08 14 06.0080000  0  2",
		"G07  23956830.530   125893980.1721       2059.717          34.000",
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Errorf("output does not contain %q\n%s", want, s)
		}
	}
	if strings.Contains(s, "SYS / PHASE SHIFT") {
		t.Errorf("output contains phase shift header\n%s", s)
	}
}

func TestReadObservationFileBlankPhaseLLI(t *testing.T) {
	obs := []SignalObservation{
		{
			T:   mustTime(t, "2025-12-17T08:11:24.9990000"),
			Sat: "G02",
			Sig: "1C",
			PR:  opt.Make(21017865.128),
			Do:  opt.Make(-289.416),
			CN0: opt.Make(float32(27)),
			LLI: opt.Make(uint8(3)),
		},
		{
			T:   mustTime(t, "2025-12-17T08:11:24.9990000"),
			Sat: "G07",
			Sig: "1C",
			PR:  opt.Make(21339746.924),
			CP:  opt.Make(112141146.362),
			Do:  opt.Make(2686.021),
			CN0: opt.Make(float32(29)),
		},
	}
	var b bytes.Buffer
	if err := WriteObservationFile(&b, Metadata{}, obs, WriterOptions{Date: time.Date(2026, time.May, 18, 9, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("WriteObservationFile: %v", err)
	}
	if !strings.Contains(b.String(), "G02  21017865.128                3       -289.416          27.000") {
		t.Fatalf("output does not contain blank phase LLI field\n%s", b.String())
	}
	_, got, err := ReadObservationFile(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("ReadObservationFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len observations = %d, want 2", len(got))
	}
	if got[0].CP.IsSet() {
		t.Errorf("CP is set to %v, want unset", got[0].CP.Get())
	}
	if got[0].LLI.Get() != 3 {
		t.Errorf("LLI = %d, want 3", got[0].LLI.Get())
	}
}
