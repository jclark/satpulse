package ubx

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func TestFindGNSS(t *testing.T) {
	extensions := []string{
		"PROTVER=18.00",
		"GPS;GLO;GAL;BDS",
		"SBAS;IMES;QZSS",
		"NAVIC",
	}

	gnssSet := findGNSS(extensions)

	expectedGNSSSet := gpsprot.MajorGNSSSet |
		gpsprot.GNSSSetOf(gpsprot.SBAS, gpsprot.QZSS, gpsprot.NAVIC)

	if gnssSet != expectedGNSSSet {
		t.Errorf("Expected GNSSSet to be %v, got %v", expectedGNSSSet, gnssSet)
	}
}

func TestOldSWVer(t *testing.T) {
	testOldSwVerString(t, "6.02 (36023)", 12, 2, 1)
	testOldSwVerString(t, "7.01 (44178)", 13, 1, 1)
}

func testOldSwVerString(t *testing.T, swVer string, major, minor byte, tmodeLevel int) {
	mv := ubxbin.MonVer{}

	copy(mv.SwVersion[:], swVer)
	ver := monVer(&mv)
	if ver.Prot.Major != major || ver.Prot.Minor != minor {
		t.Errorf("Expected ProtVer to be %d.%02d, got %d.%02d", major, minor, ver.Prot.Major, ver.Prot.Minor)
	}
	if ver.tmodeLevel() != tmodeLevel {
		t.Errorf("Expected tmodeLevel to be %d, got %d", tmodeLevel, ver.tmodeLevel())
	}
}

func TestTPIndex(t *testing.T) {
	tests := []struct {
		name string
		ver  Version
		want int
	}{
		{"ZED-X20P", testVers.x20p, 1},
		{"ZED-F9P", testVers.f9p, 0},
		{"ZED-F9T", testVers.f9t, 0},
		{"LEA-M8F", testVers.m8f, 1},
		{"NEO-M8P", testVers.m8p, 0},
		{"LEA-6T", testVers.lea6t, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ver.tpIndex(); got != tt.want {
				t.Errorf("tpIndex() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBandsConfigSupport(t *testing.T) {
	tests := []struct {
		name string
		ver  *Version
		want bool
	}{
		{"nil", nil, false},
		{"pre-gen9 HPG", &Version{Prot: &ProtVer{Major: 23, Minor: 1}, FW: &FWVer{ProductCategory: "HPG"}}, false},
		{"gen9 HPG", &Version{Prot: &ProtVer{Major: 27, Minor: 0}, FW: &FWVer{ProductCategory: "HPG"}}, true},
		{"gen9 TIM", &Version{Prot: &ProtVer{Major: 27, Minor: 0}, FW: &FWVer{ProductCategory: "TIM"}}, true},
		{"gen9 SPGL1L5", &Version{Prot: &ProtVer{Major: 34, Minor: 0}, FW: &FWVer{ProductCategory: "SPGL1L5"}}, true},
		{"gen9 SPG", &Version{Prot: &ProtVer{Major: 27, Minor: 0}, FW: &FWVer{ProductCategory: "SPG"}}, false},
		{"gen9 unknown", &Version{Prot: &ProtVer{Major: 27, Minor: 0}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ver.bandsConfigSupport(); got != tt.want {
				t.Errorf("bandsConfigSupport() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSingleBDSL1Signal(t *testing.T) {
	tests := []struct {
		name string
		ver  *Version
		want gpsprot.Signal
		ok   bool
	}{
		{
			name: "SPG",
			ver:  &Version{Prot: &ProtVer{Major: 34, Minor: 10}, FW: &FWVer{ProductCategory: "SPG"}},
			want: gpsprot.SigBDSB1I,
			ok:   true,
		},
		{
			name: "SPGL1L5",
			ver:  &Version{Prot: &ProtVer{Major: 40, Minor: 0}, FW: &FWVer{ProductCategory: "SPGL1L5"}},
			want: gpsprot.SigBDSB1C,
			ok:   true,
		},
		{
			name: "HPG",
			ver:  &Version{Prot: &ProtVer{Major: 34, Minor: 10}, FW: &FWVer{ProductCategory: "HPG"}},
		},
		{
			name: "protocol 50",
			ver:  &Version{Prot: &ProtVer{Major: 50, Minor: 0}, FW: &FWVer{ProductCategory: "SPG"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.ver.singleBDSL1Signal()
			if got != tt.want || ok != tt.ok {
				t.Errorf("got %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestVersionConfigSupport(t *testing.T) {
	tmode := gpsprot.ConfigSupportSpeed |
		gpsprot.ConfigSupportSurvey |
		gpsprot.ConfigSupportSurveyAcc |
		gpsprot.ConfigSupportFixedPos |
		gpsprot.ConfigSupportFixedPosAcc
	tmode2 := tmode | gpsprot.ConfigSupportSurveyMsg
	raw := gpsprot.ConfigSupportRaw
	msm := gpsprot.ConfigSupportRTCMMSM4 | gpsprot.ConfigSupportRTCMMSM7
	tests := []struct {
		name string
		ver  Version
		want gpsprot.ConfigSupportFlags
	}{
		{"LEA-6T", testVers.lea6t, tmode | raw},
		{"M8F", testVers.m8f, tmode2 | raw},
		{"M8P", testVers.m8p, tmode2 | raw | msm},
		{"F9P", testVers.f9p, gpsprot.ConfigSupportBand | tmode2 | raw | msm},
		{"F9T before MSM4", testVers.f9t, gpsprot.ConfigSupportBand | tmode2 | raw | gpsprot.ConfigSupportRTCMMSM7},
		{"F9T with MSM4", testVers.f9t25, gpsprot.ConfigSupportBand | tmode2 | raw | msm | gpsprot.ConfigSupportRTCMBaseID},
		{"F10S", testVers.f10s, gpsprot.ConfigSupportBand | gpsprot.ConfigSupportSpeed},
		{"X20P", testVers.x20p, gpsprot.ConfigSupportBand | tmode2 | raw | msm | gpsprot.ConfigSupportRTCMBaseID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ver.configSupport(); got != tt.want {
				t.Errorf("configSupport() = %v, want %v", got.Items(), tt.want.Items())
			}
		})
	}
}

func TestFWVerMAXF10S(t *testing.T) {
	extensions := []string{"FWVER=SPGL1L5 6.00"}
	ver := findFWVer(extensions)
	if ver == nil || ver.ProductCategory != "SPGL1L5" || ver.Major != 6 || ver.Minor != 0 {
		t.Errorf("Expected FWVer to be SPGL1L5 6.00, got %v", ver)
	}
}
