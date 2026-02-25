package gpsprot

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/decconv"
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
	return decconv.FormatInt64(int64(l), 6, 0)
}

// MarshalJSON implements json.Marshaler.
func (l Length) MarshalJSON() ([]byte, error) {
	return []byte(decconv.FormatInt64(int64(l), 6, 0)), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (l *Length) UnmarshalJSON(data []byte) error {
	v, err := decconv.ParseInt64(string(data), 6)
	if err != nil {
		return err
	}
	*l = Length(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (l Length) MarshalText() ([]byte, error) {
	return []byte(decconv.FormatInt64(int64(l), 6, 0)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (l *Length) UnmarshalText(text []byte) error {
	v, err := decconv.ParseInt64(string(text), 6)
	if err != nil {
		return err
	}
	*l = Length(v)
	return nil
}

// ParseLength parses a string as a length in meters.
func ParseLength(s string) (Length, error) {
	n, err := decconv.ParseInt64(s, 6)
	if err != nil {
		return 0, fmt.Errorf("invalid length: %q", s)
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
	return decconv.FormatInt64(int64(a), 9, 0)
}

// MarshalJSON implements json.Marshaler.
func (a Angle) MarshalJSON() ([]byte, error) {
	return []byte(decconv.FormatInt64(int64(a), 9, 0)), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (a *Angle) UnmarshalJSON(data []byte) error {
	v, err := decconv.ParseInt64(string(data), 9)
	if err != nil {
		return err
	}
	*a = Angle(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (a Angle) MarshalText() ([]byte, error) {
	return []byte(decconv.FormatInt64(int64(a), 9, 0)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (a *Angle) UnmarshalText(text []byte) error {
	v, err := decconv.ParseInt64(string(text), 9)
	if err != nil {
		return err
	}
	*a = Angle(v)
	return nil
}

// ParseAngle parses a string as an angle in degrees.
func ParseAngle(s string) (Angle, error) {
	n, err := decconv.ParseInt64(s, 9)
	if err != nil {
		return 0, fmt.Errorf("invalid angle: %q", s)
	}
	return Angle(n), nil
}

// Speed represents a speed in micrometers per second.
type Speed int64

const (
	MicrometerPerSecond Speed = 1
	MillimeterPerSecond Speed = 1000 * MicrometerPerSecond
	CentimeterPerSecond Speed = 10 * MillimeterPerSecond
	MeterPerSecond      Speed = 100 * CentimeterPerSecond
)

// MetersPerSecondFromFloat creates a Speed from a float64 value in meters per second.
func MetersPerSecondFromFloat(f float64) Speed {
	return Speed(math.Round(f * float64(MeterPerSecond)))
}

// MetersPerSecond returns the speed in meters per second as a float64.
func (s Speed) MetersPerSecond() float64 {
	return float64(s) / float64(MeterPerSecond)
}

func (s Speed) String() string {
	return decconv.FormatInt64(int64(s), 6, 0)
}

// MarshalJSON implements json.Marshaler.
func (s Speed) MarshalJSON() ([]byte, error) {
	return []byte(decconv.FormatInt64(int64(s), 6, 0)), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (sp *Speed) UnmarshalJSON(data []byte) error {
	v, err := decconv.ParseInt64(string(data), 6)
	if err != nil {
		return err
	}
	*sp = Speed(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (s Speed) MarshalText() ([]byte, error) {
	return []byte(decconv.FormatInt64(int64(s), 6, 0)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (sp *Speed) UnmarshalText(text []byte) error {
	v, err := decconv.ParseInt64(string(text), 6)
	if err != nil {
		return err
	}
	*sp = Speed(v)
	return nil
}

// Duration represents a duration that serialises as decimal seconds.
type Duration time.Duration

const (
	Nanosecond  Duration = Duration(time.Nanosecond)
	Microsecond Duration = Duration(time.Microsecond)
	Millisecond Duration = Duration(time.Millisecond)
	Second      Duration = Duration(time.Second)
)

// Seconds creates a Duration from a float64 value in seconds.
func Seconds(f float64) Duration {
	return Duration(math.Round(f * float64(Second)))
}

// Seconds returns the duration in seconds as a float64.
func (d Duration) Seconds() float64 {
	return time.Duration(d).Seconds()
}

func (d Duration) String() string {
	return decconv.FormatInt64(int64(d), 9, 0)
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(decconv.FormatInt64(int64(d), 9, 0)), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(data []byte) error {
	v, err := decconv.ParseInt64(string(data), 9)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(decconv.FormatInt64(int64(d), 9, 0)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := decconv.ParseInt64(string(text), 9)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// ParseDuration parses a string as a duration in seconds.
func ParseDuration(s string) (Duration, error) {
	n, err := decconv.ParseInt64(s, 9)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %q", s)
	}
	return Duration(n), nil
}

// Point3D represents a 3D point with Length coordinates.
type Point3D [3]Length

func (p Point3D) String() string {
	return decconv.FormatInt64(int64(p[0]), 6, 0) + "," +
		decconv.FormatInt64(int64(p[1]), 6, 0) + "," +
		decconv.FormatInt64(int64(p[2]), 6, 0)
}

func (p Point3D) IsZero() bool {
	return p[0] == 0 && p[1] == 0 && p[2] == 0
}

// ParsePoint3D parses a string as three comma-separated coordinates in meters.
func ParsePoint3D(s string) (Point3D, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return Point3D{}, fmt.Errorf("invalid 3D coordinates: %q", s)
	}
	var p Point3D
	for i, part := range parts {
		n, err := decconv.ParseInt64(part, 6)
		if err != nil {
			return Point3D{}, fmt.Errorf("invalid 3D coordinates: %q", s)
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
