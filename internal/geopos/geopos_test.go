package geopos

import (
	"math"
	"testing"
)

// CheckECEFOnEarth function as defined earlier

// TestCheckOnEarthValid tests valid ECEF positions of known landmarks
func TestCheckOnEarthValid(t *testing.T) {
	validPositions := []ECEF{
		{3978578.17, -8652.15, 4968410.94},    // Big Ben
		{4200935.82, 168323.10, 4780213.04},   // Eiffel Tower
		{1331340.65, -4656583.35, 4136313.40}, // Statue of Liberty
		{0.0, 0.0, 6356752.31},                // North Pole
		{0.0, 0.0, -6356752.31},               // South Pole
		{302769.89, 5636025.47, 2979493.09},   // Mount Everest
	}

	for _, pos := range validPositions {
		if err := pos.CheckOnEarth(); err != nil {
			t.Errorf("Valid position failed: %v, error: %v", pos, err)
		}
	}
}

var landmarks = []LLA{
	{40.689247, -74.044502, 0},     // Statue of Liberty, New York, USA
	{48.858222, 2.2945, 0},         // Eiffel Tower, Paris, France
	{27.175, 78.042222, 0},         // Taj Mahal, Agra, India
	{51.500729, -0.124625, 0},      // Big Ben, London, UK
	{30.328611, 35.444444, 0},      // Petra, Jordan
	{35.658611, 139.745556, 0},     // Tokyo Tower, Tokyo, Japan
	{55.752222, 37.615556, 0},      // The Kremlin, Moscow, Russia
	{30.251667, -97.751944, 0},     // Texas State Capitol, Austin, Texas, USA
	{35.360556, 138.727778, 3776},  // Mount Fuji, Japan
	{-22.951944, -43.210556, 0},    // Christ the Redeemer, Rio de Janeiro, Brazil
	{-13.163333, -72.545556, 2430}, // Machu Picchu, Peru
	{25.197139, 55.274111, 0},      // Burj Khalifa, Dubai, UAE
	{37.819722, -122.478611, 0},    // Golden Gate Bridge, San Francisco, USA
	{20.682778, -88.568611, 0},     // Chichen Itza, Yucatán, Mexico
	{1.286389, 103.854444, 0},      // Marina Bay Sands, Singapore
	{41.890169, 12.492269, 0},      // Colosseum, Rome, Italy
	{52.516278, 13.377722, 0},      // Brandenburg Gate, Berlin, Germany
	{-33.856784, 151.215297, 0},    // Sydney Opera House, Sydney, Australia
	{27.988056, 86.925278, 8848},   // Mount Everest, Nepal/China border
}

func TestRoundtip(t *testing.T) {
	const tolerance = 1e-6
	for _, lla := range landmarks {
		ecef := WGS84.LLAtoECEF(lla)
		lla2 := WGS84.ECEFtoLLA(ecef)
		d := max(math.Abs(lla.Lat-lla2.Lat), math.Abs(lla.Lon-lla2.Lon), math.Abs(lla.Alt-lla2.Alt))
		if d > tolerance {
			t.Errorf("Roundtrip failed: %v, %v (max diff %f)", lla, lla2, d)
		}
	}
}

func TestCheckOnEarthValidLLA(t *testing.T) {
	for _, lla := range landmarks {
		ecef := WGS84.LLAtoECEF(lla)
		if err := ecef.CheckOnEarth(); err != nil {
			t.Errorf("Valid position failed: %v, error: %v", lla, err)
		}
	}
}

// TestCheckOnEarthInvalid tests invalid ECEF positions
func TestCheckOnEarthInvalid(t *testing.T) {
	invalidPositions := []ECEF{
		{7e6, 0, 0},     // Beyond maximum range
		{0, 7e6, 0},     // Beyond maximum range
		{0, 0, -7e6},    // Beyond maximum range
		{0, 0, 0},       // Center of earth
		{0, 0, 6e6},     // Below minimum range
		{8e6, 8e6, 8e6}, // Far outside valid range
	}

	for _, pos := range invalidPositions {
		if err := pos.CheckOnEarth(); err == nil {
			t.Errorf("Invalid position passed: %v", pos)
		}
	}
}

// TestLLAtoECEF tests the LLAtoECEF function with known input and expected output.
func TestLLAtoECEF(t *testing.T) {
	// Test data
	lla := LLA{51.477928, -0.001545, 0.0} // Coordinates for the Royal Observatory, Greenwich

	// Expected ECEF coordinates (approximate)
	expected := [3]float64{3980570.07, -107.34, 4966833.39}

	// Tolerance for floating-point comparison
	const tolerance = 1e-2

	// Perform conversion
	ecef := WGS84.LLAtoECEF(lla)

	// Check each component of the result against the expected values
	for i, v := range ecef {
		if math.Abs(v-expected[i]) > tolerance {
			t.Errorf("LLAtoECEF(%v)[%d] = %f, want %f", lla, i, v, expected[i])
		}
	}
}
