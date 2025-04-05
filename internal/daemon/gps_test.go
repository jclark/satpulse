package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestGPSConfig(t *testing.T) {
	cfgStr := `[gps]
	config = true
	antennaCableLength = 20
	GNSS="galileo"
	fixedPosAcc = 5
	fixedPosECEF = [3978578.17, -8652.15, 4968410.94]`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	target, err := cfg.GPS.target()
	if err != nil {
		t.Fatal(err)
	}
	cp := &target.Props
	delay, _ := cp.GetAntennaCableDelay()
	if (delay - time.Nanosecond*20*5).Abs() > time.Nanosecond {
		t.Errorf("antennaCableDelay: got %v, want about 100ns", delay)
	}
	gnss, _ := cp.GetPrimaryGNSS()
	if gnss != gpsprot.GAL {
		t.Errorf("GNSS: got %v, want %v", gnss, gpsprot.GAL)
	}
	acc, _ := cp.GetFixedPosAcc()
	if acc != gpsprot.Meter*5 {
		t.Errorf("fixedPosAcc: got %v, want 5m", acc)
	}
	ecef, _ := cp.GetFixedPosECEF()
	if ecef.String() != "3978578.17,-8652.15,4968410.94" {
		t.Errorf("fixedPosECEF: got %v, want 3978578.17,-8652.15,4968410.94", ecef)
	}
}
