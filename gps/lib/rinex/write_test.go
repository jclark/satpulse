package rinex

import (
	"bytes"
	"fmt"
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
			SignalValues: SignalValues{
				PR:  opt.Make(23956830.530),
				CP:  opt.Make(125893980.172),
				Do:  opt.Make(2059.717),
				CN0: opt.Make(float32(34)),
				LLI: opt.Make(LLILostLock),
			},
		},
		{
			T:   mustTime(t, "2025-12-17T08:14:06.0080000"),
			Sat: "R06",
			Sig: "1C",
			SignalValues: SignalValues{
				Frq: opt.Make(int8(-4)),
				PR:  opt.Make(24968868.310),
				CP:  opt.Make(133238671.287),
				Do:  opt.Make(-2790.338),
				CN0: opt.Make(float32(32)),
			},
		},
	}
	var b bytes.Buffer
	meta := Metadata{Run: MetadataRun{Program: "test", Date: time.Date(2026, time.May, 18, 9, 0, 0, 0, time.UTC)}}
	if err := WriteObservationFile(&b, meta, obs); err != nil {
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

func TestWriteObservationFileUsesGPSTimeSystem(t *testing.T) {
	gpsT := mustTime(t, "2026-05-19T13:31:24.0000000")
	for _, tt := range []struct {
		name       string
		obs        []SignalObservation
		want       []string
		wantEpochs int
	}{
		{
			name: "pure GPS",
			obs: []SignalObservation{
				{T: gpsT, Sat: "G03", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
			},
			want: []string{
				"OBSERVATION DATA    G: GPS",
				"2026    05    19    13    31   24.0000000     GPS         TIME OF FIRST OBS",
				"> 2026 05 19 13 31 24.0000000  0  1",
			},
			wantEpochs: 1,
		},
		{
			name: "pure GLONASS",
			obs: []SignalObservation{
				{T: gpsT, Sat: "R05", Sig: "1C", SignalValues: SignalValues{Frq: opt.Make(int8(1)), PR: opt.Make(1.0)}},
			},
			want: []string{
				"OBSERVATION DATA    R: GLONASS",
				"2026    05    19    13    31   24.0000000     GPS         TIME OF FIRST OBS",
				"> 2026 05 19 13 31 24.0000000  0  1",
			},
			wantEpochs: 1,
		},
		{
			name: "pure Galileo",
			obs: []SignalObservation{
				{T: gpsT, Sat: "E11", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
			},
			want: []string{
				"OBSERVATION DATA    E: Galileo",
				"2026    05    19    13    31   24.0000000     GPS         TIME OF FIRST OBS",
				"> 2026 05 19 13 31 24.0000000  0  1",
			},
			wantEpochs: 1,
		},
		{
			name: "pure BDS",
			obs: []SignalObservation{
				{T: gpsT, Sat: "C06", Sig: "2I", SignalValues: SignalValues{PR: opt.Make(1.0)}},
			},
			want: []string{
				"OBSERVATION DATA    C: BDS",
				"2026    05    19    13    31   24.0000000     GPS         TIME OF FIRST OBS",
				"> 2026 05 19 13 31 24.0000000  0  1",
			},
			wantEpochs: 1,
		},
		{
			name: "pure QZSS",
			obs: []SignalObservation{
				{T: gpsT, Sat: "J01", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
			},
			want: []string{
				"OBSERVATION DATA    J: QZSS",
				"2026    05    19    13    31   24.0000000     GPS         TIME OF FIRST OBS",
				"> 2026 05 19 13 31 24.0000000  0  1",
			},
			wantEpochs: 1,
		},
		{
			name: "pure NavIC",
			obs: []SignalObservation{
				{T: gpsT, Sat: "I05", Sig: "5A", SignalValues: SignalValues{PR: opt.Make(1.0)}},
			},
			want: []string{
				"OBSERVATION DATA    I: NavIC",
				"2026    05    19    13    31   24.0000000     GPS         TIME OF FIRST OBS",
				"> 2026 05 19 13 31 24.0000000  0  1",
			},
			wantEpochs: 1,
		},
		{
			name: "mixed GPS",
			obs: []SignalObservation{
				{T: gpsT, Sat: "G03", Sig: "1C", SignalValues: SignalValues{PR: opt.Make(1.0)}},
				{T: gpsT, Sat: "R05", Sig: "1C", SignalValues: SignalValues{Frq: opt.Make(int8(1)), PR: opt.Make(1.0)}},
			},
			want: []string{
				"OBSERVATION DATA    M: Mixed",
				"2026    05    19    13    31   24.0000000     GPS         TIME OF FIRST OBS",
				"> 2026 05 19 13 31 24.0000000  0  2",
				"G03         1.000",
				"R05         1.000",
			},
			wantEpochs: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			meta := Metadata{LeapSeconds: testPtr(int16(18)), Run: MetadataRun{Date: time.Date(2026, time.May, 19, 0, 0, 0, 0, time.UTC)}}
			if err := WriteObservationFile(&b, meta, tt.obs); err != nil {
				t.Fatalf("WriteObservationFile: %v", err)
			}
			s := b.String()
			for _, want := range tt.want {
				if !strings.Contains(s, want) {
					t.Errorf("output does not contain %q\n%s", want, s)
				}
			}
			if got := strings.Count(s, "\n> "); got != tt.wantEpochs {
				t.Errorf("epoch count = %d, want %d\n%s", got, tt.wantEpochs, s)
			}
		})
	}
}

func TestReadObservationFileTimeSystems(t *testing.T) {
	for _, tt := range []struct {
		name            string
		timeSystem      string
		omitTimeSystem  bool
		omitLeapSeconds bool
		obsTypes        []string
		satLines        []string
		want            map[SatelliteID]string
	}{
		{
			name:       "pure GPS",
			timeSystem: "GPS",
			obsTypes:   []string{"G    1 C1C"},
			satLines:   []string{"G03           1.000"},
			want:       map[SatelliteID]string{"G03": "2026-05-19T13:31:24.0000000"},
		},
		{
			name:       "pure GLONASS",
			timeSystem: "GLO",
			obsTypes:   []string{"R    1 C1C"},
			satLines:   []string{"R05           1.000"},
			want:       map[SatelliteID]string{"R05": "2026-05-19T13:31:24.0000000"},
		},
		{
			name:           "pure GLONASS default time system",
			timeSystem:     "GLO",
			omitTimeSystem: true,
			obsTypes:       []string{"R    1 C1C"},
			satLines:       []string{"R05           1.000"},
			want:           map[SatelliteID]string{"R05": "2026-05-19T13:31:24.0000000"},
		},
		{
			name:            "pure GLONASS default leap seconds",
			timeSystem:      "GLO",
			omitLeapSeconds: true,
			obsTypes:        []string{"R    1 C1C"},
			satLines:        []string{"R05           1.000"},
			want:            map[SatelliteID]string{"R05": "2026-05-19T13:31:24.0000000"},
		},
		{
			name:       "pure BDS",
			timeSystem: "BDT",
			obsTypes:   []string{"C    1 C2I"},
			satLines:   []string{"C06           1.000"},
			want:       map[SatelliteID]string{"C06": "2026-05-19T13:31:24.0000000"},
		},
		{
			name:       "pure Galileo",
			timeSystem: "GAL",
			obsTypes:   []string{"E    1 C1C"},
			satLines:   []string{"E11           1.000"},
			want:       map[SatelliteID]string{"E11": "2026-05-19T13:31:24.0000000"},
		},
		{
			name:       "pure QZSS",
			timeSystem: "QZS",
			obsTypes:   []string{"J    1 C1C"},
			satLines:   []string{"J01           1.000"},
			want:       map[SatelliteID]string{"J01": "2026-05-19T13:31:24.0000000"},
		},
		{
			name:       "pure NavIC",
			timeSystem: "IRN",
			obsTypes:   []string{"I    1 C5A"},
			satLines:   []string{"I05           1.000"},
			want:       map[SatelliteID]string{"I05": "2026-05-19T13:31:24.0000000"},
		},
		{
			name:       "mixed GPS",
			timeSystem: "GPS",
			obsTypes:   []string{"G    1 C1C", "R    1 C1C"},
			satLines:   []string{"G03           1.000", "R05           1.000"},
			want: map[SatelliteID]string{
				"G03": "2026-05-19T13:31:24.0000000",
				"R05": "2026-05-19T13:31:24.0000000",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, obs, err := ReadObservationFile(strings.NewReader(testObservationFile(tt.timeSystem, !tt.omitTimeSystem, !tt.omitLeapSeconds, tt.obsTypes, tt.satLines)))
			if err != nil {
				t.Fatalf("ReadObservationFile: %v", err)
			}
			got := make(map[SatelliteID]Time)
			for _, o := range obs {
				got[o.Sat] = o.T
			}
			for sat, want := range tt.want {
				if got[sat].String() != want {
					t.Errorf("%s time = %s, want %s", sat, got[sat], want)
				}
			}
		})
	}
}

func TestReadObservationFileBlankPhaseLLI(t *testing.T) {
	obs := []SignalObservation{
		{
			T:   mustTime(t, "2025-12-17T08:11:24.9990000"),
			Sat: "G02",
			Sig: "1C",
			SignalValues: SignalValues{
				PR:  opt.Make(21017865.128),
				Do:  opt.Make(-289.416),
				CN0: opt.Make(float32(27)),
				LLI: opt.Make(LLILostLock | LLIHalfCycleAmbiguity),
			},
		},
		{
			T:   mustTime(t, "2025-12-17T08:11:24.9990000"),
			Sat: "G07",
			Sig: "1C",
			SignalValues: SignalValues{
				PR:  opt.Make(21339746.924),
				CP:  opt.Make(112141146.362),
				Do:  opt.Make(2686.021),
				CN0: opt.Make(float32(29)),
			},
		},
	}
	var b bytes.Buffer
	meta := Metadata{Run: MetadataRun{Date: time.Date(2026, time.May, 18, 9, 0, 0, 0, time.UTC)}}
	if err := WriteObservationFile(&b, meta, obs); err != nil {
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
	if got[0].LLI.Get() != LLILostLock|LLIHalfCycleAmbiguity {
		t.Errorf("LLI = %d, want 3", got[0].LLI.Get())
	}
}

func TestWriteObservationFileZeroIndicators(t *testing.T) {
	obs := []SignalObservation{
		{
			T:   mustTime(t, "2025-12-17T08:14:06.0080000"),
			Sat: "G07",
			Sig: "1C",
			SignalValues: SignalValues{
				CP:  opt.Make(1.0),
				LLI: opt.Make(LLI(0)),
				SSI: opt.Make(uint8(0)),
			},
		},
	}
	var b bytes.Buffer
	meta := Metadata{Run: MetadataRun{Date: time.Date(2026, time.May, 18, 9, 0, 0, 0, time.UTC)}}
	if err := WriteObservationFile(&b, meta, obs); err != nil {
		t.Fatalf("WriteObservationFile: %v", err)
	}
	if !strings.Contains(b.String(), "G07         1.00000") {
		t.Fatalf("output does not contain explicit zero indicators\n%s", b.String())
	}
	_, got, err := ReadObservationFile(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("ReadObservationFile: %v", err)
	}
	if !got[0].LLI.IsSet() || got[0].LLI.Get() != 0 {
		t.Errorf("LLI = %v, want explicit 0", got[0].LLI)
	}
	if !got[0].SSI.IsSet() || got[0].SSI.Get() != 0 {
		t.Errorf("SSI = %v, want explicit 0", got[0].SSI)
	}
}

func testObservationFile(timeSystem string, includeTimeSystem, includeLeapSeconds bool, obsTypes, satLines []string) string {
	var b strings.Builder
	b.WriteString(testHeaderLine("     3.04           OBSERVATION DATA    M: Mixed", "RINEX VERSION / TYPE"))
	for _, s := range obsTypes {
		b.WriteString(testHeaderLine(s, "SYS / # / OBS TYPES"))
	}
	timeHeader := fmt.Sprintf("  2026    05    19    13    31   %s.0000000", testSecond(timeSystem))
	if includeTimeSystem {
		timeHeader = fmt.Sprintf("%s     %-3s", timeHeader, timeSystem)
	}
	b.WriteString(testHeaderLine(timeHeader, "TIME OF FIRST OBS"))
	if includeLeapSeconds {
		b.WriteString(testHeaderLine("    18", "LEAP SECONDS"))
	}
	b.WriteString(testHeaderLine("", "END OF HEADER"))
	b.WriteString(fmt.Sprintf("> 2026 05 19 13 31 %s.0000000  0%3d\n", testSecond(timeSystem), len(satLines)))
	for _, s := range satLines {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return b.String()
}

func testSecond(timeSystem string) string {
	// Non-GPS labels are chosen so conversion to GPS yields second 24.
	if timeSystem == "GLO" {
		return "06"
	}
	if timeSystem == "BDT" {
		return "10"
	}
	return "24"
}

func testHeaderLine(content, label string) string {
	return fmt.Sprintf("%-60s%-20s\n", content, label)
}
