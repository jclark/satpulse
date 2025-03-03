package mon

import (
	"math"
	"math/rand"
	"testing"

	"github.com/jclark/satpulse/internal/ptime"
)

func TestAvgFilterWarmup(t *testing.T) {
	// Create filter with alpha = 0.1
	alpha := 0.1
	filter := newAvgFilter(alpha)
	era := ptime.Era(1)

	warmupVal := 20.0
	// Fill warmup buffer with constant value
	warmupSize := warmupCount(alpha)
	for i := 0; i < warmupSize; i++ {
		result := filter.add(warmupVal, era)
		// With constant input, output should equal input
		if !closeEnough(result, warmupVal) {
			t.Errorf("With constant input %f, expected output %f, got %f", warmupVal, warmupVal, result)
		}
	}

	// After warmup, initVals should be nil
	if filter.initVals != nil {
		t.Errorf("initVals should be nil after warmup")
	}
}

func TestAvgFilterSmoothing(t *testing.T) {
	// Create filter with alpha = 0.1
	alpha := 0.1
	filter := newAvgFilter(alpha)
	era := ptime.Era(1)

	// First complete warmup with constant values
	warmupSize := warmupCount(alpha)
	for i := 0; i < warmupSize; i++ {
		filter.add(20.0, era)
	}

	// Generate test sequence:
	// - Random values between -30 and 30
	// - Occasional spikes that last for multiple samples
	seed := int64(42) // Fixed seed for reproducibility
	rng := rand.New(rand.NewSource(seed))

	const testLength = 1000
	inputs := make([]float64, testLength)

	// Track if we're currently in a spike period and for how many more samples
	spikeRemaining := 0
	currentSpike := 0.0

	for i := 0; i < testLength; i++ {
		// If we're in a spike period, continue with the same spike value
		if spikeRemaining > 0 {
			inputs[i] = currentSpike
			spikeRemaining--
		} else if rng.Intn(100) < 5 { // 5% chance of starting a spike
			// Start a new spike that lasts 2-8 samples
			spikeRemaining = rng.Intn(7) + 2 // 2 to 8 samples

			// Determine spike direction and value
			sign := 1
			if rng.Intn(2) == 0 {
				sign = -1
			}

			// Generate a consistent spike value to use for the entire spike period
			currentSpike = float64(sign) * (50.0 + rng.Float64()*50.0) // Spike between 50-100
			inputs[i] = currentSpike
			spikeRemaining--
		} else {
			// Normal value between -30 and 30
			inputs[i] = (rng.Float64()*60.0 - 30.0)
		}
	}

	// Process through the filter
	outputs := make([]float64, testLength)
	var maxDiff float64 = 0

	for i, input := range inputs {
		outputs[i] = filter.add(input, era)

		if i > 0 {
			// Calculate maximum step between consecutive outputs
			diff := math.Abs(outputs[i] - outputs[i-1])
			if diff > maxDiff {
				maxDiff = diff
			}
		}
	}

	// Verify smoothing properties:

	// 1. Check that max step in output is less than max step in input
	var maxInputStep float64 = 0
	for i := 1; i < len(inputs); i++ {
		diff := math.Abs(inputs[i] - inputs[i-1])
		if diff > maxInputStep {
			maxInputStep = diff
		}
	}

	t.Logf("Max input step: %.2f, Max output step: %.2f", maxInputStep, maxDiff)
	if maxDiff >= maxInputStep {
		t.Errorf("Filter output should have smaller steps than input. Max input step: %.2f, max output step: %.2f",
			maxInputStep, maxDiff)
	}

	// 2. Verify output standard deviation is less than input standard deviation
	inputStdDev := calculateStdDev(inputs)
	outputStdDev := calculateStdDev(outputs)

	t.Logf("Input std dev: %.2f, Output std dev: %.2f", inputStdDev, outputStdDev)
	if outputStdDev >= inputStdDev {
		t.Errorf("Filter should reduce standard deviation. Input: %.2f, Output: %.2f",
			inputStdDev, outputStdDev)
	}

	// 3. Ensure extreme spikes are attenuated
	var maxInputVal, maxOutputVal float64 = 0, 0
	for i := 0; i < len(inputs); i++ {
		if math.Abs(inputs[i]) > maxInputVal {
			maxInputVal = math.Abs(inputs[i])
		}
		if math.Abs(outputs[i]) > maxOutputVal {
			maxOutputVal = math.Abs(outputs[i])
		}
	}

	t.Logf("Max input: %.2f, Max output: %.2f", maxInputVal, maxOutputVal)
	if maxOutputVal >= maxInputVal {
		t.Errorf("Filter should attenuate extreme values. Max input: %.2f, Max output: %.2f",
			maxInputVal, maxOutputVal)
	}

	// 4. Check that mean is approximately preserved
	inputMean := calculateMean(inputs)
	outputMean := calculateMean(outputs)

	meanDiff := math.Abs(inputMean - outputMean)
	t.Logf("Input mean: %.2f, Output mean: %.2f, Difference: %.2f", inputMean, outputMean, meanDiff)
	if meanDiff > 10.0 { // Allow some difference but not excessive
		t.Errorf("Filter should approximately preserve mean. Input mean: %.2f, Output mean: %.2f",
			inputMean, outputMean)
	}
}

func TestAvgFilterSustainedChange(t *testing.T) {
	// Create filter with alpha = 0.1
	alpha := 0.1
	filter := newAvgFilter(alpha)
	era := ptime.Era(1)

	// Complete warmup with zeros
	warmupSize := warmupCount(alpha)
	for i := 0; i < warmupSize; i++ {
		filter.add(0.0, era)
	}

	// Apply a sustained large value and check that filter follows
	largeValue := 80.0

	// Apply the value for sufficient time to see convergence
	var outputs []float64
	samples := 30
	for i := 0; i < samples; i++ {
		output := filter.add(largeValue, era)
		outputs = append(outputs, output)
	}

	// Log a few values to see the progression
	t.Logf("Sustained value: %.2f", largeValue)
	t.Logf("Output after 1 sample: %.2f", outputs[0])
	t.Logf("Output after 10 samples: %.2f", outputs[9])
	t.Logf("Output after 20 samples: %.2f", outputs[19])
	t.Logf("Output after 30 samples: %.2f", outputs[29])

	// After sufficient time, filter should have moved significantly toward the target
	if outputs[29] < largeValue*0.9 {
		t.Errorf("Filter should follow sustained changes. After 30 samples with target %.2f, output was only %.2f",
			largeValue, outputs[29])
	}

	// Output should monotonically approach the target
	for i := 1; i < len(outputs); i++ {
		if outputs[i] <= outputs[i-1] {
			t.Errorf("Filter output should increase monotonically when following sustained positive change. "+
				"Sample %d: %.2f, Sample %d: %.2f", i-1, outputs[i-1], i, outputs[i])
			break
		}
	}
}

func TestAvgFilterEraChange(t *testing.T) {
	alpha := 0.1
	filter := newAvgFilter(alpha)
	era1 := ptime.Era(1)

	// Fill warmup and add a few more values
	warmupSize := warmupCount(alpha)
	for i := 0; i < warmupSize+5; i++ {
		filter.add(20.0, era1)
	}

	// Verify we're out of warmup
	if filter.initVals != nil {
		t.Errorf("Should be out of warmup mode")
	}

	// Change era
	era2 := ptime.Era(2)
	result := filter.add(50.0, era2)

	// First result should be the input
	if result != 50.0 {
		t.Errorf("Expected first result after era change to be 50.0, got %f", result)
	}

	// Should be back in warmup mode
	if filter.initVals == nil {
		t.Errorf("Should be back in warmup mode after era change")
	}

	// Length of initVals should be 1
	if len(filter.initVals) != 1 {
		t.Errorf("Expected initVals length to be 1, got %d", len(filter.initVals))
	}
}

// Helper function to calculate standard deviation
func calculateStdDev(values []float64) float64 {
	mean := calculateMean(values)

	var sumSquaredDiff float64 = 0
	for _, v := range values {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}

	return math.Sqrt(sumSquaredDiff / float64(len(values)))
}

// Helper function to calculate mean
func calculateMean(values []float64) float64 {
	var sum float64 = 0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// Helper function for floating point comparisons
func closeEnough(a, b float64) bool {
	const epsilon = 1e-10
	return math.Abs(a-b) < epsilon
}
