package ubx

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
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
		gpsprot.GNSSFlag(gpsprot.SBAS, gpsprot.QZSS, gpsprot.NAVIC)

	if gnssSet != expectedGNSSSet {
		t.Errorf("Expected GNSSSet to be %v, got %v", expectedGNSSSet, gnssSet)
	}
}

func TestOldSWVer(t *testing.T) {
	testOldSwVerString(t, "6.02 (36023)", 12, 2, 1)
	testOldSwVerString(t, "7.01 (44178)", 13, 1, 1)
}

func testOldSwVerString(t *testing.T, swVer string, major, minor byte, tmodeLevel int) {
	mv := bin.MonVer{}

	copy(mv.SwVersion[:], swVer)
	ver := monVer(&mv)
	if ver.Prot.Major != major || ver.Prot.Minor != minor {
		t.Errorf("Expected ProtVer to be %d.%02d, got %d.%02d", major, minor, ver.Prot.Major, ver.Prot.Minor)
	}
	if ver.tmodeLevel() != tmodeLevel {
		t.Errorf("Expected tmodeLevel to be %d, got %d", tmodeLevel, ver.tmodeLevel())
	}
}

func TestFWVerMAXF10S(t *testing.T) {
	extensions := []string{ "FWVER=SPGL1L5 6.00" }
	ver := findFWVer(extensions)
	if ver == nil || ver.ProductCategory != "SPGL1L5" || ver.Major != 6 || ver.Minor != 0 {
		t.Errorf("Expected FWVer to be SPGL1L5 6.00, got %v", ver)
	}
}