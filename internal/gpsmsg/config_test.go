package gpsmsg

import (
	"testing"
	"time"
)

func TestConfigDB(t *testing.T) {
	cm := ConfigMap{}

	// Set some values using CfgKey
	CfgSolutionPeriod.Set(&cm, 10*time.Second)
	CfgTimePulseWidth.Set(&cm, 1*time.Millisecond)
	CfgTimePulseAlignToGNSS.Set(&cm, true)

	keys := []CfgKey{CfgSolutionPeriod, CfgTimePulseWidth, CfgTimePulseAlignToGNSS}
	nonKeys := []CfgKey{CfgTimePulsePeriod, CfgStationary}

	// Get the values using CfgKey
	solutionPeriod, ok := CfgSolutionPeriod.Get(&cm)
	if !ok {
		t.Errorf("expected CfgSolutionPeriod to be set")
	}
	if solutionPeriod != 10*time.Second {
		t.Errorf("expected CfgSolutionPeriod to be 10s, got %v", solutionPeriod)
	}

	timePulseWidth, ok := CfgTimePulseWidth.Get(&cm)
	if !ok {
		t.Errorf("expected CfgTimePulseWidth to be set")
	}
	if timePulseWidth != 1*time.Millisecond {
		t.Errorf("expected CfgTimePulseWidth to be 1ms, got %v", timePulseWidth)
	}

	timePulsePeriod, ok := CfgTimePulsePeriod.Get(&cm)
	if ok {
		t.Errorf("expected CfgTimePulsePeriod not to be set")
	}
	if timePulsePeriod != 0 {
		t.Errorf("expected CfgTimePulsePeriod to be 0, got %v", timePulsePeriod)
	}

	timePulseGNSS, ok := CfgTimePulseAlignToGNSS.Get(&cm)
	if !ok {
		t.Errorf("expected CfgTimePulseAlignGNSS to be set")
	}
	if timePulseGNSS != true {
		t.Error("expected CfgTimePulseGNSS to be true, got false")
	}

	for _, k := range keys {
		if !cm.Contains(k) {
			t.Errorf("expected %v to be set", k)
		}
	}
	for _, k := range nonKeys {
		if cm.Contains(k) {
			t.Errorf("expected %v not to be set", k)
		}
	}

}

func TestPoint3DRoundTrip(t *testing.T) {
	// Typical ECEF coordinates for New York
	originalPoint := Point3D{1334195 * Meter, -4652309 * Meter, 4138066 * Meter}

	pointStr := originalPoint.String()

	parsedPoint, err := ParsePoint3D(pointStr)
	if err != nil {
		t.Fatalf("ParsePoint3D returned error: %v", err)
	}

	for i := 0; i < 3; i++ {
		if parsedPoint[i] != originalPoint[i] {
			t.Errorf("Expected coordinate %d to be %v, got %v", i, originalPoint[i], parsedPoint[i])
		}
	}
	p, err := ParsePoint3D("1,2,3 ")

	if err != nil {
		t.Errorf("ParsePoint3D returned an error for trailing space")
	} else {
		if p.String() != "1,2,3" {
			t.Errorf("ParsePoint3D returned %v for trailing space", p.String())
		}
	}
}
