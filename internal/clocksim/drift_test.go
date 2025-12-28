package clocksim

import (
	"math"
	"testing"
)

func TestDriftPhiElements(t *testing.T) {
	// Test case from Python: omega_n=0.002, zeta=0.7, dt=1.0
	omegaN := 0.002
	zeta := 0.7
	dt := 1.0
	phi11, phi12, phi21, phi22 := DriftPhiElements(omegaN, zeta, dt)

	// Python reference values
	wantPhi11 := 9.999980018660265e-01
	wantPhi12 := 9.986006400185282e-01
	wantPhi21 := -3.994402560074112e-06
	wantPhi22 := 9.972019200739748e-01

	tol := 1e-12
	if math.Abs(phi11-wantPhi11) > tol {
		t.Errorf("phi11: got %e, want %e", phi11, wantPhi11)
	}
	if math.Abs(phi12-wantPhi12) > tol {
		t.Errorf("phi12: got %e, want %e", phi12, wantPhi12)
	}
	if math.Abs(phi21-wantPhi21) > tol {
		t.Errorf("phi21: got %e, want %e", phi21, wantPhi21)
	}
	if math.Abs(phi22-wantPhi22) > tol {
		t.Errorf("phi22: got %e, want %e", phi22, wantPhi22)
	}
}

func TestDriftUnitPhaseVariance(t *testing.T) {
	tests := []struct {
		name    string
		omegaN  float64
		zeta    float64
		wantVar float64
	}{
		{"tau=500,zeta=0.7", 0.002, 0.7, 4.464285714282776e+07},
		{"tau=5000,zeta=0.7", 0.0002, 0.7, 4.464285714289423e+10},
		{"tau=1000,zeta=0.5", 0.001, 0.5, 5.000000000000590e+08},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DriftUnitPhaseVariance(tc.omegaN, tc.zeta, 1.0)
			relErr := math.Abs(got-tc.wantVar) / tc.wantVar
			if relErr > 1e-9 {
				t.Errorf("got %e, want %e (rel err %e)", got, tc.wantVar, relErr)
			}
		})
	}
}

func TestDriftUserToInternal(t *testing.T) {
	tests := []struct {
		name           string
		tau, sigma     float64
		zeta           float64
		wantOmegaN     float64
		wantZeta       float64
		wantSigmaDrift float64
	}{
		{
			name: "tau=500,sigma=10,zeta=0.7",
			tau: 500.0, sigma: 10.0, zeta: 0.7,
			wantOmegaN:     2.000000000000000e-03,
			wantZeta:       7.000000000000000e-01,
			wantSigmaDrift: 1.496662954710069e-12,
		},
		{
			name: "tau=5000,sigma=20,zeta=0.7",
			tau: 5000.0, sigma: 20.0, zeta: 0.7,
			wantOmegaN:     2.000000000000000e-04,
			wantZeta:       7.000000000000000e-01,
			wantSigmaDrift: 9.465727652955453e-14,
		},
		{
			name: "tau=1000,sigma=15,zeta=0.5",
			tau: 1000.0, sigma: 15.0, zeta: 0.5,
			wantOmegaN:     1.000000000000000e-03,
			wantZeta:       5.000000000000000e-01,
			wantSigmaDrift: 6.708203932498974e-13,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			omegaN, zeta, sigmaDrift := DriftUserToInternal(tc.tau, tc.sigma, tc.zeta)
			if math.Abs(omegaN-tc.wantOmegaN) > 1e-15 {
				t.Errorf("omegaN: got %e, want %e", omegaN, tc.wantOmegaN)
			}
			if math.Abs(zeta-tc.wantZeta) > 1e-15 {
				t.Errorf("zeta: got %e, want %e", zeta, tc.wantZeta)
			}
			relErr := math.Abs(sigmaDrift-tc.wantSigmaDrift) / tc.wantSigmaDrift
			if relErr > 1e-9 {
				t.Errorf("sigmaDrift: got %e, want %e (rel err %e)", sigmaDrift, tc.wantSigmaDrift, relErr)
			}
		})
	}
}

func TestDriftUserToInternalZeroInputs(t *testing.T) {
	// Zero tau should return zeros
	omegaN, zeta, sigmaDrift := DriftUserToInternal(0, 10.0, 0.7)
	if omegaN != 0 || zeta != 0 || sigmaDrift != 0 {
		t.Errorf("zero tau: got (%e, %e, %e), want all zeros", omegaN, zeta, sigmaDrift)
	}
	// Zero sigma should return zeros
	omegaN, zeta, sigmaDrift = DriftUserToInternal(500.0, 0, 0.7)
	if omegaN != 0 || zeta != 0 || sigmaDrift != 0 {
		t.Errorf("zero sigma: got (%e, %e, %e), want all zeros", omegaN, zeta, sigmaDrift)
	}
}

func TestDriftUserToInternalDefaultZeta(t *testing.T) {
	// Zero zeta should use default 0.7
	omegaN, zeta, sigmaDrift := DriftUserToInternal(500.0, 10.0, 0)
	if zeta != 0.7 {
		t.Errorf("zeta: got %e, want 0.7", zeta)
	}
	// Should produce same result as explicit 0.7
	omegaN2, zeta2, sigmaDrift2 := DriftUserToInternal(500.0, 10.0, 0.7)
	if omegaN != omegaN2 || zeta != zeta2 || sigmaDrift != sigmaDrift2 {
		t.Errorf("default zeta mismatch: (%e,%e,%e) vs (%e,%e,%e)",
			omegaN, zeta, sigmaDrift, omegaN2, zeta2, sigmaDrift2)
	}
}

func TestDriftRoundTrip(t *testing.T) {
	// Test that user -> internal -> user produces the original values
	tests := []struct {
		tau, sigma, zeta float64
	}{
		{500.0, 10.0, 0.7},
		{5000.0, 20.0, 0.7},
		{1000.0, 15.0, 0.5},
		{3000.0, 25.0, 0.9},
	}
	for _, tc := range tests {
		omegaN, zeta, sigmaDrift := DriftUserToInternal(tc.tau, tc.sigma, tc.zeta)
		tauBack, sigmaBack, zetaBack := DriftInternalToUser(omegaN, zeta, sigmaDrift)

		if math.Abs(tauBack-tc.tau) > 0.001 {
			t.Errorf("tau roundtrip: %f -> %f", tc.tau, tauBack)
		}
		relErr := math.Abs(sigmaBack-tc.sigma) / tc.sigma
		if relErr > 1e-9 {
			t.Errorf("sigma roundtrip: %f -> %f (rel err %e)", tc.sigma, sigmaBack, relErr)
		}
		if math.Abs(zetaBack-tc.zeta) > 1e-15 {
			t.Errorf("zeta roundtrip: %f -> %f", tc.zeta, zetaBack)
		}
	}
}
