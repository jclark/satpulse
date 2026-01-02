package allan

import (
	"math"
	"testing"
	"time"
)

// This data was generated using allantools Python package.
// import numpy as np
// import allantools
// x = allantools.noise.white(100)
// np.set_printoptions(precision=17)
// (taus, adevs, errors, ns) = allantools.oadev(x)
// Then cut and paste the values of x and adevs
var testPhaseData = []float64{
	-0.4751206333780582, 1.520184206553047, -0.14772926288572885,
	0.16310709178447927, 0.43826188191402776, -0.2187647294101627,
	-0.0841906894882181, 0.44913863915456753, -0.14782898731461683,
	1.1236622192051715, -0.19978768961792456, -0.10187475790062255,
	-0.06255338916224294, -0.47956753334807695, -0.31207622849804945,
	0.13116900185040345, 0.15430700352117685, 0.1367841963431773,
	-0.6028895506484775, -0.8840434513639386, 0.7981834927739838,
	-0.5475302248321033, -0.30694114163106895, -1.0640631564961742,
	-0.7754329676406091, 0.04061239177745107, 0.06317044665133692,
	0.8802118030195435, 0.2792441139840559, 0.8886889832150038,
	-0.07441188573395069, -0.44845937637009026, -0.1525076941144072,
	-0.09291493581323865, -0.9946284850913676, -1.2716236828076697,
	0.32927499371353414, -0.35348549570905163, 1.2470260641275812,
	-0.20103535019327845, -0.6899523723792568, -1.8501033824318882,
	0.3263271685211205, -0.2891859574827033, -0.8167031103126828,
	-0.03157150824640836, -0.8778039843338128, -0.4001480419133279,
	-0.1915222895046719, -0.2919741626530488, 0.696172358562121,
	-0.1962001797672767, -0.07082660756031862, 0.10617887588503097,
	0.9867449479446941, -0.8967131940896254, 1.700463424041293,
	0.7694417578502705, 1.1713476615563059, 0.09264995598685014,
	0.35955034644528416, -1.173310555939686, 0.7127271647347839,
	0.0758008885287637, 1.3748756612958024, -0.04243967301018474,
	0.1414548125707525, -0.18116878233218356, 0.9085929386153522,
	-0.6080754634224734, -0.7490145928499617, 0.37476609153166623,
	-1.362395625827447, 0.2156329454146618, 1.3687185161408901,
	-1.105654239014437, 1.3600037777005842, 0.28902613730145355,
	-0.6951718988399128, -0.6550985288028234, -0.2941227354855268,
	-0.8526728222157982, 0.6660743867240676, 0.21916238480788533,
	-0.3971333014511144, -0.7859447374602139, 0.0864133265872225,
	-0.373504092716328, 0.3971702711403118, -0.01743829038644755,
	0.3609165958251858, 0.7780919707988202, -0.38714271241134535,
	0.44238581244696384, 0.09835314700583894, -0.01892933897896386,
	-0.16121757922741475, 0.6560448226709198, -0.43035492503655637,
	0.9499164543456045,
}

var testAdevs = []float64{
	1.269865897684586, 0.5142161359895249, 0.315359613867583,
	0.14134375719257675, 0.08437589810874188, 0.04103699973312587,
}

func TestOverlapADev(t *testing.T) {

	m := 1
	for _, adev := range testAdevs {
		got := OverlapADev(testPhaseData, 1.0, m)
		if math.Abs(got-adev) > 1e-15 {
			t.Errorf("OverlapADev() = %v, want %v", got, adev)
		}
		m *= 2
	}

	durations := make([]time.Duration, len(testPhaseData))
	for i, f := range testPhaseData {
		durations[i] = time.Duration(f * float64(time.Second))
	}
	m = 1
	for _, adev := range testAdevs {
		got := OverlapADev(durations, time.Second, m)
		if math.Abs(got-adev) > 1e-9 {
			t.Errorf("OverlapADevDurations() = %v, want %v", got, adev)
		}
		m *= 2
	}
	// Test with empty phase data
	emptyData := []float64{}
	if !math.IsNaN(OverlapADev(emptyData, 1.0, 1)) {
		t.Errorf("OverlapADev() with empty data should return NaN")
	}

	// Test with insufficient length relative to m
	insufficientData := []float64{1.0, 2.0, 3.0, 4.0} // Length is less than 2*m for m > 2
	for m := 3; m <= 5; m++ {
		if !math.IsNaN(OverlapADev(insufficientData, 1.0, m)) {
			t.Errorf("OverlapADev() with insufficient data length for m=%v should return NaN", m)
		}
	}
}

func TestAccum(t *testing.T) {
	// Compare incremental Accum against batch OverlapADev for m=1
	acc := NewAccum(1.0)
	for _, v := range testPhaseData {
		acc.Update(v)
	}
	got := acc.ADev()
	want := OverlapADev(testPhaseData, 1.0, 1)
	if math.Abs(got-want) > 1e-15 {
		t.Errorf("Accum.ADev() = %v, want %v", got, want)
	}

	// Test with insufficient samples
	acc2 := NewAccum(1.0)
	if !math.IsNaN(acc2.ADev()) {
		t.Error("Accum.ADev() with no samples should return NaN")
	}
	acc2.Update(1.0)
	acc2.Update(2.0)
	if !math.IsNaN(acc2.ADev()) {
		t.Error("Accum.ADev() with only 2 samples should return NaN")
	}
	acc2.Update(3.0)
	if math.IsNaN(acc2.ADev()) {
		t.Error("Accum.ADev() with 3 samples should not return NaN")
	}
}
