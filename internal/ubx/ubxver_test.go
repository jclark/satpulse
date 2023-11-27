package ubx

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestFindGNSS(t *testing.T) {
	extensions := []string{
		"PROTVER=18.00",
		"GPS;GLO;GAL;BDS",
		"SBAS;IMES;QZSS",
	}

	gnssSet := findGNSS(extensions)

	expectedGNSSSet := gpsprot.MajorGNSSSet |
		gpsprot.GNSSFlag(gpsprot.SBAS, gpsprot.QZSS)

	if gnssSet != expectedGNSSSet {
		t.Errorf("Expected GNSSSet to be %v, got %v", expectedGNSSSet, gnssSet)
	}
}
