package gpsprot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Length represents a length in micrometers.
type Length int64

const (
	Micrometer Length = 1
	Millimeter Length = 1000 * Micrometer
	Centimeter Length = 10 * Millimeter
	Meter      Length = 100 * Centimeter
)

// Meters creates a Length from a float64 value in meters.
func Meters(f float64) Length {
	return Length(math.Round(f * float64(Meter)))
}

// Meters returns the length in meters as a float64.
func (l Length) Meters() float64 {
	return float64(l) / float64(Meter)
}

func (l Length) String() string {
	return fmt.Sprintf("%v", l.Meters())
}

// ParseLength parses a string as a length in meters.
func ParseLength(s string) (Length, error) {
	var f float64
	var trailing string
	if n, err := fmt.Sscanf(s, "%f%s", &f, &trailing); n != 1 || err != io.EOF {
		return 0, fmt.Errorf("invalid length: %q", s)
	}
	n, err := float64ToInt64(math.Round(f * float64(Meter)))
	if err != nil {
		return 0, fmt.Errorf("invalid length %f: %w", f, err)
	}
	return Length(n), nil
}

// Angle represents an angle in nanodegrees.
type Angle int64

const (
	Nanodegrees  Angle = 1
	Microdegrees Angle = 1000 * Nanodegrees
	Millidegrees Angle = 1000 * Microdegrees
	Degrees      Angle = 1000 * Millidegrees
)

// DegreesFromFloat creates an Angle from a float64 value in degrees.
func DegreesFromFloat(f float64) Angle {
	return Angle(math.Round(f * float64(Degrees)))
}

// Degrees returns the angle in degrees as a float64.
func (a Angle) Degrees() float64 {
	return float64(a) / float64(Degrees)
}

func (a Angle) String() string {
	return fmt.Sprintf("%v", a.Degrees())
}

// ParseAngle parses a string as an angle in degrees.
func ParseAngle(s string) (Angle, error) {
	var f float64
	var trailing string
	if n, err := fmt.Sscanf(s, "%f%s", &f, &trailing); n != 1 || err != io.EOF {
		return 0, fmt.Errorf("invalid angle: %q", s)
	}
	n, err := float64ToInt64(math.Round(f * float64(Degrees)))
	if err != nil {
		return 0, fmt.Errorf("invalid angle %f: %w", f, err)
	}
	return Angle(n), nil
}

// Point3D represents a 3D point with Length coordinates.
type Point3D [3]Length

func (p Point3D) String() string {
	var s [3]string
	for i := 0; i < 3; i++ {
		s[i] = strconv.FormatFloat(p[i].Meters(), 'f', -1, 64)
	}
	return fmt.Sprintf("%s,%s,%s", s[0], s[1], s[2])
}

func (p Point3D) IsZero() bool {
	return p[0] == 0 && p[1] == 0 && p[2] == 0
}

// ParsePoint3D parses a string as three comma-separated coordinates in meters.
func ParsePoint3D(s string) (Point3D, error) {
	var p Point3D
	var trailing string
	var f [3]float64
	if n, err := fmt.Sscanf(s, "%f,%f,%f%s", &f[0], &f[1], &f[2], &trailing); n != 3 || err != io.EOF {
		return p, fmt.Errorf("invalid 3D coordinates: %q", s)
	}
	for i := 0; i < 3; i++ {
		n, err := float64ToInt64(math.Round(f[i] * float64(Meter)))
		if err != nil {
			return Point3D{}, fmt.Errorf("invalid coordinate %f: %w", f[i], err)
		}
		p[i] = Length(n)
	}
	return p, nil
}

//go:generate stringer -type=GNSS
type GNSS uint8

// Constants for GNSS type.
// Zero value means invalid/unknown/unspecified.
// The major GNSS systems are first.
// SBAS is an augmentation system, and not a standalone GNSS system.
const (
	GPS      GNSS = iota + 1 // GPS (USA)
	GAL                      // Galileo (Europe)
	BDS                      // BeiDou (China)
	GLO                      // GLONASS (Russia)
	QZSS                     // QZSS (Japan)
	NAVIC                    // NavIC (India)
	SBAS                     // Satellite-Based Augmentation System (e.g. WAAS, EGNOS, GAGAN, MSAS)
	GNSSLast GNSS = SBAS
)

// ParseGNSS parses a GNSS name string into a GNSS value.
func ParseGNSS(s string) (GNSS, error) {
	switch strings.ToUpper(s) {
	case "GPS":
		return GPS, nil
	case "GAL", "GALILEO":
		return GAL, nil
	case "BDS", "BEIDOU":
		return BDS, nil
	case "GLO", "GLONASS":
		return GLO, nil
	case "NAVIC":
		return NAVIC, nil
	case "QZSS":
		return QZSS, nil
	case "SBAS":
		return SBAS, nil
	}
	if s == "" {
		return 0, errors.New("invalid GNSS name: empty string")
	}
	return 0, fmt.Errorf("%s: invalid GNSS name", s)
}

// SVIDPrefix returns the single-letter prefix used in SVID string representation.
func (g GNSS) SVIDPrefix() string {
	switch g {
	case GPS:
		return "G"
	case GAL:
		return "E"
	case BDS:
		return "C"
	case GLO:
		return "R"
	case NAVIC:
		return "I"
	case QZSS:
		return "J"
	case SBAS:
		return "S"
	default:
		return ""
	}
}

// IsValid returns true if the GNSS value is a known system.
func (g GNSS) IsValid() bool {
	return g > 0 && g <= GNSSLast
}

// IsMajor returns true if the GNSS is one of the four major systems (GPS, Galileo, BeiDou, GLONASS).
func (g GNSS) IsMajor() bool {
	return g >= GPS && g <= GLO
}

func (g GNSS) MarshalJSON() ([]byte, error) {
	return json.Marshal(g.String())
}

func (g GNSS) MarshalText() ([]byte, error) {
	return []byte(g.String()), nil
}

// There are 24 operational GLONASS satellites with slot numbers 1 to 24.
// But there can be others that are spares or in testing.
// See https://glonass-iac.ru/en/sostavOG which is referenced by
// https://files.igs.org/pub/resource/working_groups/multi_gnss/Metadata_SINEX_1.10.pdf
// So it is possible to have satellite numbers for GLONASS > 24.
// Maximum number of spares there has ever been is 3.
// NMEA allows up to 8 which would imply a total of 32 slots.
// This matches the use of 5 bits for slot number in the GLONASS ICD,
// so it seems like a sensible upper limit.
const MaxSpareGLONASS = 8

// IsValidSVNum checks if the given SV number is valid for the GNSS type.
// Numbers are as in RINEX 3.04.
func (g GNSS) IsValidSVNum(num int) bool {
	if num < 1 {
		return false
	}
	switch g {
	case GPS:
		return num <= 32
	case GLO:
		return num <= 24+MaxSpareGLONASS
	case GAL:
		return num <= 36
	case BDS:
		return num <= 63
	case QZSS:
		return num <= 10
	case NAVIC:
		return num <= 14
	case SBAS:
		return num >= 20 && num <= 58
	default:
		return false
	}
}

func (gp *GNSS) UnmarshalText(text []byte) error {
	g, err := ParseGNSS(string(text))
	if err == nil {
		*gp = g
	}
	return err
}
