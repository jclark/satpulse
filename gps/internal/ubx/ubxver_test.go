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

func TestFWVerMAXF10S(t *testing.T) {
	extensions := []string{"FWVER=SPGL1L5 6.00"}
	ver := findFWVer(extensions)
	if ver == nil || ver.ProductCategory != "SPGL1L5" || ver.Major != 6 || ver.Minor != 0 {
		t.Errorf("Expected FWVer to be SPGL1L5 6.00, got %v", ver)
	}
}
