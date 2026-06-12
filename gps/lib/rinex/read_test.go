package rinex

import (
	"fmt"
	"strings"
	"testing"
)

func TestReadObservationFileEpochFlags(t *testing.T) {
	rnx := testRINEXObservationFile(
		[]string{"G    1 C1C"},
		nil,
		[]string{
			testEpochLine(1, 1, "24"),
			"G03           1.000",
			testBlankEpochLine(2, 2),
			"start moving antenna",
			"still not an observation",
			testBlankEpochLine(3, 1),
			"new site occupation",
			testBlankEpochLine(5, 1),
			"external event",
			testEpochLine(6, 1, "25"),
			"G04           9.000",
			testEpochLine(0, 1, "26"),
			"G04           2.000",
		})
	_, got, err := ReadObservationFile(strings.NewReader(rnx))
	if err != nil {
		t.Fatalf("ReadObservationFile: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len observations = %d, want 2: %#v", len(got), got)
	}
	if got[0].Sat != "G03" || !got[0].PR.IsSet() || got[0].PR.Get() != 1 {
		t.Errorf("first observation = %#v, want G03 C1C 1.000", got[0])
	}
	if got[1].Sat != "G04" || !got[1].PR.IsSet() || got[1].PR.Get() != 2 {
		t.Errorf("second observation = %#v, want G04 C1C 2.000", got[1])
	}
}

func TestReadObservationFileRejectsUnsupportedEpochFlags(t *testing.T) {
	for _, tt := range []struct {
		name string
		flag int
		want string
	}{
		{name: "mid-file header", flag: 4, want: "unsupported mid-file header update"},
		{name: "unknown flag", flag: 9, want: "unsupported epoch flag 9"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rnx := testRINEXObservationFile([]string{"G    1 C1C"}, nil, []string{testBlankEpochLine(tt.flag, 1), "payload"})
			_, _, err := ReadObservationFile(strings.NewReader(rnx))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ReadObservationFile error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestReadObservationFileSkippedEpochUnexpectedEOF(t *testing.T) {
	rnx := testRINEXObservationFile([]string{"G    1 C1C"}, nil, []string{testBlankEpochLine(2, 1)})
	_, _, err := ReadObservationFile(strings.NewReader(rnx))
	if err == nil || !strings.Contains(err.Error(), "unexpected EOF in event records") {
		t.Fatalf("ReadObservationFile error = %v, want unexpected EOF in event records", err)
	}
}

func TestReadObservationFileRejectsObservationAffectingHeaders(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		label   string
		want    string
	}{
		{name: "scale factor", content: "G    1 10 C1C", label: "SYS / SCALE FACTOR", want: "unsupported SYS / SCALE FACTOR header"},
		{name: "signal strength unit", content: "SNR", label: "SIGNAL STRENGTH UNIT", want: "unsupported signal strength unit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rnx := testRINEXObservationFile([]string{"G    1 C1C"}, []string{testHeaderLine(tt.content, tt.label)}, nil)
			_, _, err := ReadObservationFile(strings.NewReader(rnx))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ReadObservationFile error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestReadObservationFileAcceptsDBHZSignalStrengthUnit(t *testing.T) {
	rnx := testRINEXObservationFile(
		[]string{"G    1 S1C"},
		[]string{testHeaderLine("DBHZ", "SIGNAL STRENGTH UNIT")},
		[]string{
			testEpochLine(0, 1, "24"),
			"G03          45.000",
		})
	_, got, err := ReadObservationFile(strings.NewReader(rnx))
	if err != nil {
		t.Fatalf("ReadObservationFile: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len observations = %d, want 1", len(got))
	}
	if !got[0].CN0.IsSet() || got[0].CN0.Get() != 45 {
		t.Errorf("CN0 = %v, want 45", got[0].CN0)
	}
}

func testRINEXObservationFile(obsTypes, headers, body []string) string {
	var b strings.Builder
	b.WriteString(testHeaderLine("     3.04           OBSERVATION DATA    M: Mixed", "RINEX VERSION / TYPE"))
	for _, s := range obsTypes {
		b.WriteString(testHeaderLine(s, "SYS / # / OBS TYPES"))
	}
	b.WriteString(testHeaderLine("  2026    05    19    13    31   24.0000000     GPS", "TIME OF FIRST OBS"))
	b.WriteString(testHeaderLine("    18", "LEAP SECONDS"))
	for _, s := range headers {
		b.WriteString(s)
	}
	b.WriteString(testHeaderLine("", "END OF HEADER"))
	for _, s := range body {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return b.String()
}

func testEpochLine(flag, n int, sec string) string {
	return fmt.Sprintf("> 2026 05 19 13 31 %s.0000000  %d%3d", sec, flag, n)
}

func testBlankEpochLine(flag, n int) string {
	return fmt.Sprintf(">%30s%d%3d", "", flag, n)
}
