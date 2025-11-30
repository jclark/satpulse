package clocksim

import "math"

// DriftPhiElements computes the discrete state transition matrix elements
// for the Carpenter-Lee 2nd-order Gauss-Markov drift process.
//
// For the underdamped case (zeta < 1), computes the state transition matrix
// elements for the discrete-time update:
//
//	[x_{k+1}]   [phi11  phi12] [x_k]
//	[v_{k+1}] = [phi21  phi22] [v_k]
//
// where x is phase and v is velocity.
func DriftPhiElements(omegaN, zeta, dt float64) (phi11, phi12, phi21, phi22 float64) {
	// Clamp zeta to ensure underdamped behaviour
	zetaClamped := min(zeta, 0.99)
	omegaD := omegaN * math.Sqrt(1.0-zetaClamped*zetaClamped)
	decay := math.Exp(-zetaClamped * omegaN * dt)
	cosWd := math.Cos(omegaD * dt)
	sinWd := math.Sin(omegaD * dt)

	phi11 = decay * (cosWd + (zetaClamped*omegaN/omegaD)*sinWd)
	phi12 = decay * (1.0 / omegaD) * sinWd
	phi21 = decay * (-(omegaN * omegaN / omegaD) * sinWd)
	phi22 = decay * (cosWd - (zetaClamped*omegaN/omegaD)*sinWd)
	return
}

// SolveDiscreteLyapunov2x2 solves the discrete Lyapunov equation for a 2x2 system:
//
//	P = Phi @ P @ Phi^T + Q
//
// Using the Kronecker product approach:
//
//	vec(P) = (I - Phi ⊗ Phi)^(-1) @ vec(Q)
//
// where vec() stacks columns and ⊗ is Kronecker product.
// For 2x2, this is a 4x4 linear system.
//
// Returns the 2x2 solution matrix P as [p00, p01, p10, p11].
func SolveDiscreteLyapunov2x2(phi11, phi12, phi21, phi22, q00, q01, q10, q11 float64) (p00, p01, p10, p11 float64) {
	// Build I - Phi ⊗ Phi (4x4)
	// Phi ⊗ Phi where Phi = [[a,b],[c,d]] gives:
	// [[a*Phi, b*Phi],
	//  [c*Phi, d*Phi]]
	// = [[a*a, a*b, b*a, b*b],
	//    [a*c, a*d, b*c, b*d],
	//    [c*a, c*b, d*a, d*b],
	//    [c*c, c*d, d*c, d*d]]

	a, b, c, d := phi11, phi12, phi21, phi22

	// A = I - Phi ⊗ Phi
	// Row 0: I[0,0]=1, others 0
	// Row 1: I[1,1]=1
	// Row 2: I[2,2]=1
	// Row 3: I[3,3]=1

	a00 := 1 - a*a
	a01 := -a * b
	a02 := -b * a
	a03 := -b * b
	a10 := -a * c
	a11 := 1 - a*d
	a12 := -b * c
	a13 := -b * d
	a20 := -c * a
	a21 := -c * b
	a22 := 1 - d*a
	a23 := -d * b
	a30 := -c * c
	a31 := -c * d
	a32 := -d * c
	a33 := 1 - d*d

	// vec(Q) = [q00, q10, q01, q11] (column-major)
	rhs0 := q00
	rhs1 := q10
	rhs2 := q01
	rhs3 := q11

	// Solve 4x4 system using Gaussian elimination with partial pivoting
	// Working in-place on augmented matrix
	m := [4][5]float64{
		{a00, a01, a02, a03, rhs0},
		{a10, a11, a12, a13, rhs1},
		{a20, a21, a22, a23, rhs2},
		{a30, a31, a32, a33, rhs3},
	}

	// Forward elimination with partial pivoting
	for col := 0; col < 4; col++ {
		// Find pivot
		maxRow := col
		maxVal := math.Abs(m[col][col])
		for row := col + 1; row < 4; row++ {
			if math.Abs(m[row][col]) > maxVal {
				maxVal = math.Abs(m[row][col])
				maxRow = row
			}
		}
		// Swap rows
		if maxRow != col {
			m[col], m[maxRow] = m[maxRow], m[col]
		}
		// Check for singular matrix
		if math.Abs(m[col][col]) < 1e-15 {
			return 0, 0, 0, 0
		}
		// Eliminate below
		for row := col + 1; row < 4; row++ {
			factor := m[row][col] / m[col][col]
			for k := col; k < 5; k++ {
				m[row][k] -= factor * m[col][k]
			}
		}
	}

	// Back substitution
	x := [4]float64{}
	for i := 3; i >= 0; i-- {
		x[i] = m[i][4]
		for j := i + 1; j < 4; j++ {
			x[i] -= m[i][j] * x[j]
		}
		x[i] /= m[i][i]
	}

	// vec(P) = [p00, p10, p01, p11]
	p00 = x[0]
	// p10 := x[1] - for symmetric P, p01 should equal p10
	p01 = x[2]
	p11 = x[3]
	return p00, p01, p01, p11
}

// DriftUnitPhaseVariance returns Var(x) for a unit-intensity discrete drift block.
//
// Solves the discrete Lyapunov equation:
//
//	P = Phi @ P @ Phi^T + Q_unit
//
// where Q_unit = [[0, 0], [0, 1]] (unit variance process noise on velocity only).
// Returns P[0,0], the stationary variance of the phase state.
func DriftUnitPhaseVariance(omegaN, zeta, dt float64) float64 {
	phi11, phi12, phi21, phi22 := DriftPhiElements(omegaN, zeta, dt)
	// Q_unit = [[0, 0], [0, 1]]
	p00, _, _, _ := SolveDiscreteLyapunov2x2(phi11, phi12, phi21, phi22, 0, 0, 0, 1)
	return p00
}

// DriftUserToInternal converts user-facing drift parameters to internal simulator parameters.
//
// Uses discrete Lyapunov calibration so that driftSigma matches the actual RMS
// of the 1 Hz discrete drift simulator output.
//
// The discrete system uses Q = [[0,0],[0,sigma_drift² * dt]], so:
//
//	actual_variance = sigma_drift² * dt * var_x_unit
//
// where var_x_unit is the phase variance for unit noise intensity.
//
// Parameters:
//
//	driftTau: Timescale in seconds (1/omega_n)
//	driftSigmaNs: RMS phase in nanoseconds
//	driftZeta: Damping ratio (dimensionless, default 0.7)
//
// Returns (omegaN, zeta, sigmaDrift) where:
//
//	omegaN: Natural frequency (1/s)
//	zeta: Damping ratio (unchanged)
//	sigmaDrift: Noise intensity for simulator (seconds)
func DriftUserToInternal(driftTau, driftSigmaNs, driftZeta float64) (omegaN, zeta, sigmaDrift float64) {
	if driftTau <= 0 || driftSigmaNs <= 0 {
		return 0, 0, 0
	}
	omegaN = 1.0 / driftTau
	zeta = driftZeta
	if zeta <= 0 {
		zeta = 0.7 // default damping ratio
	}
	driftSigmaSec := driftSigmaNs * 1e-9

	// dt = 1.0 for 1 Hz sampling
	dt := 1.0
	varXUnit := DriftUnitPhaseVariance(omegaN, zeta, dt)

	// actual_variance = sigma_drift² * dt * var_x_unit = drift_sigma_sec²
	// sigma_drift = drift_sigma_sec / sqrt(dt * var_x_unit)
	if varXUnit > 0 {
		sigmaDrift = driftSigmaSec / math.Sqrt(dt*varXUnit)
	}
	return omegaN, zeta, sigmaDrift
}

// DriftInternalToUser converts internal simulator parameters to user-facing parameters.
//
// Inverse of DriftUserToInternal, for serialisation and reporting.
//
// Parameters:
//
//	omegaN: Natural frequency (1/s)
//	zeta: Damping ratio
//	sigmaDrift: Noise intensity (seconds)
//
// Returns (driftTau, driftSigmaNs, driftZeta).
func DriftInternalToUser(omegaN, zeta, sigmaDrift float64) (driftTau, driftSigmaNs, driftZeta float64) {
	if omegaN <= 0 || sigmaDrift <= 0 {
		return 0, 0, zeta
	}
	driftTau = 1.0 / omegaN
	driftZeta = zeta

	dt := 1.0
	varXUnit := DriftUnitPhaseVariance(omegaN, zeta, dt)
	// var_x = sigma_drift² * dt * var_x_unit
	varX := sigmaDrift * sigmaDrift * dt * varXUnit
	if varX > 0 {
		driftSigmaNs = math.Sqrt(varX) * 1e9
	}
	return driftTau, driftSigmaNs, driftZeta
}

