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
	cm := &target.Map
	delay, _ := gpsprot.CfgAntennaCableDelay.Get(cm)
	if (delay - time.Nanosecond*20*5).Abs() > time.Nanosecond {
		t.Errorf("antennaCableDelay: got %v, want about 100ns", delay)
	}
	gnss, _ := gpsprot.CfgPrimaryGNSS.Get(cm)
	if gnss != gpsprot.GAL {
		t.Errorf("GNSS: got %v, want %v", gnss, gpsprot.GAL)
	}
	acc, _ := gpsprot.CfgFixedPosAcc.Get(cm)
	if acc != gpsprot.Meter*5 {
		t.Errorf("fixedPosAcc: got %v, want 5m", acc)
	}
	ecef, _ := gpsprot.CfgFixedPosECEF.Get(cm)
	if ecef.String() != "3978578.17,-8652.15,4968410.94" {
		t.Errorf("fixedPosECEF: got %v, want 3978578.17,-8652.15,4968410.94", ecef)
	}
}
