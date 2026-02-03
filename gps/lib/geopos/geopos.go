package geopos

import (
	"fmt"
	"math"
)

type ECEF [3]float64

// CheckOnEarth checks if the given ECEF coordinates are within a reasonable range for a position on Earth
func (xyz ECEF) CheckOnEarth() error {
	const (
		radiusMax = 6.39e6 // Equatorial radius plus height of Mount Everest, rounded up
		radiusMin = 6.35e6 // Radius at poles
	)

	for i, coord := range xyz {
		if math.Abs(coord) > radiusMax {
			return fmt.Errorf("coordinate index %d with value %f is out of range", i, coord)
		}
	}

	m := xyz.magnitude()
	if m < radiusMin {
		return fmt.Errorf("%v: total ECEF vector magnitude is below the minimum expected for Earth", xyz)
	}
	if m > radiusMax {
		return fmt.Errorf("%v: total ECEF vector magnitude exceeds the maximum expected for Earth", xyz)
	}
	return nil
}

func (xyz ECEF) magnitude() float64 {
	return math.Sqrt(xyz[0]*xyz[0] + xyz[1]*xyz[1] + xyz[2]*xyz[2])
}

func (xyz ECEF) IsZero() bool {
	return xyz[0] == 0 && xyz[1] == 0 && xyz[2] == 0
}

// LLH represents latitude, longitude, and height.
type LLH struct {
	Lat, Lon, Height float64 // Lat, Lon in degrees; Height above the ellipsoid in meters
}

// WGS84 constants
const wgs84a = 6378137.0                       // Semi-major axis
const wgs84invF = 298.257223563                // Inverse flattening
const wgs84f = 1 / wgs84invF                   // Flattening
const wgs84eSquared = 2*wgs84f - wgs84f*wgs84f // Eccentricity squared

const radiansPerDegree = math.Pi / 180

type wgs84 byte

// WGS84 is a constant representing the WGS84 datum.
// It provides methods converting between LLA and ECEF coordinates.
const WGS84 wgs84 = 0

// LLHtoECEF converts latitude, longitude, and height to ECEF coordinates using WGS84 datum.
func (wgs84) LLHtoECEF(llh LLH) ECEF {
	// Convert degrees to radians
	lat := llh.Lat * radiansPerDegree
	lon := llh.Lon * radiansPerDegree
	// Intermediate calculation (prime vertical radius of curvature)
	r := wgs84a / math.Sqrt(1-wgs84eSquared*math.Sin(lat)*math.Sin(lat))
	return ECEF{
		(r + llh.Height) * math.Cos(lat) * math.Cos(lon),
		(r + llh.Height) * math.Cos(lat) * math.Sin(lon),
		((1-wgs84eSquared)*r + llh.Height) * math.Sin(lat),
	}
}

const degreesPerRadian = 180 / math.Pi

// ECEFtoLLH converts ECEF coordinates to LLH (Latitude, Longitude, Height) using WGS84 datum.
// ecef is a [3]float64 array representing ECEF coordinates in meters.
// Returns LLH struct with latitude and longitude in degrees, height in meters.
func (wgs84) ECEFtoLLH(ecef ECEF) (LLH, error) {
	err := ecef.CheckOnEarth()
	if err != nil {
		return LLH{}, err
	}
	x, y, z := ecef[0], ecef[1], ecef[2]

	lon := math.Atan2(y, x) * degreesPerRadian

	// Calculate hypotenuse from the ECEF x-y plane
	h := math.Sqrt(x*x + y*y)

	// Initial approximation of latitude (assuming altitude is 0)
	lat := math.Atan2(z, h*(1-wgs84eSquared))
	latPrev := 0.0

	const tolerance = 1e-12 // Convergence tolerance

	for {
		latPrev = lat
		r := wgs84a / math.Sqrt(1-wgs84eSquared*square(math.Sin(lat)))
		alt := h/math.Cos(lat) - r
		lat = math.Atan2(z, h*(1-wgs84eSquared*(r/(r+alt))))
		// write it like this to bulletproof against lat becoming NaN
		if !(math.Abs(lat-latPrev) > tolerance) {
			return LLH{Lat: lat * degreesPerRadian, Lon: lon, Height: alt}, nil
		}
	}
}

func square(x float64) float64 {
	return x * x
}
