package configcmd

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsmsg"
)

func TestGnssListSet(t *testing.T) {
	gl := &gnssList{}
	err := gl.Set("GPS,GLO")
	if err != nil {
		t.Errorf("Set returned error: %v", err)
	}

	if len(gl.gnss) != 2 {
		t.Errorf("Expected length of 2, got %v", len(gl.gnss))
	}

	if gl.gnss[0] != gpsmsg.GPS {
		t.Errorf("Expected first element to be GPS, got %v", gl.gnss[0])
	}

	if gl.gnss[1] != gpsmsg.GLO {
		t.Errorf("Expected second element to be GLO, got %v", gl.gnss[1])
	}

	err = gl.Set("")
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	if err.Error() != "invalid GNSS name: empty string" {
		t.Errorf("Unexpected error message for empty string, got %v", err.Error())
	}
}
