package rinex

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
)

type observationHeader struct {
	meta       Metadata
	codes      map[string][]ObservationCode
	frq        map[SatelliteID]int8
	sys        string
	timeSystem string
}

// ReadObservationFile reads a RINEX observation file.
func ReadObservationFile(r io.Reader) (Metadata, []SignalObservation, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 1024), 1024*1024)
	h, err := readObservationHeader(s)
	if err != nil {
		return Metadata{}, nil, err
	}
	obs, err := readObservationEpochs(s, h)
	if err != nil {
		return Metadata{}, nil, err
	}
	return h.meta, obs, nil
}

func readObservationHeader(s *bufio.Scanner) (observationHeader, error) {
	h := observationHeader{
		codes: make(map[string][]ObservationCode),
		frq:   make(map[SatelliteID]int8),
	}
	obsTypeCount := make(map[string]int)
	for s.Scan() {
		line := padLine(s.Text(), 80)
		content := line[:60]
		label := strings.TrimSpace(line[60:80])
		switch label {
		case "RINEX VERSION / TYPE":
			fields := strings.Fields(content[:20])
			if len(fields) != 0 {
				h.meta.Version = Version(fields[0])
			}
		case "PGM / RUN BY / DATE":
			h.meta.Run.Program = strings.TrimSpace(content[:20])
			h.meta.Run.By = strings.TrimSpace(content[20:40])
			h.meta.Run.Date = parseRunDate(content[40:60])
		case "COMMENT":
			h.meta.Comment = append(h.meta.Comment, strings.TrimRight(content, " "))
		case "MARKER NAME":
			h.meta.Marker.Name = strings.TrimSpace(content)
		case "MARKER NUMBER":
			h.meta.Marker.Number = strings.TrimSpace(content)
		case "MARKER TYPE":
			h.meta.Marker.Type = strings.TrimSpace(content)
		case "OBSERVER / AGENCY":
			h.meta.Observer = strings.TrimSpace(content[:20])
			h.meta.Agency = strings.TrimSpace(content[20:])
		case "REC # / TYPE / VERS":
			h.meta.Receiver.Number = strings.TrimSpace(content[:20])
			h.meta.Receiver.Type = strings.TrimSpace(content[20:40])
			h.meta.Receiver.Version = strings.TrimSpace(content[40:])
		case "ANT # / TYPE":
			h.meta.Antenna.Number = strings.TrimSpace(content[:20])
			h.meta.Antenna.Type = strings.TrimSpace(content[20:40])
		case "APPROX POSITION XYZ":
			if v, ok := parseFloatTriple(content); ok {
				h.meta.ApproxPosition = &v
			}
		case "ANTENNA: DELTA H/E/N":
			if v, ok := parseFloatTriple(content); ok {
				h.meta.AntennaDelta = &v
			}
		case "SYS / # / OBS TYPES":
			sys, err := readObsTypesHeader(content, h.sys, h.codes, obsTypeCount)
			if err != nil {
				return h, err
			}
			h.sys = sys
		case "INTERVAL":
			fields := strings.Fields(content)
			if len(fields) != 0 {
				v, err := strconv.ParseFloat(fields[0], 64)
				if err != nil {
					return h, fmt.Errorf("rinex: invalid interval %q", fields[0])
				}
				h.meta.Interval = &v
			}
		case "GLONASS SLOT / FRQ #":
			if err := readGLONASSFreqHeader(content, h.frq); err != nil {
				return h, err
			}
		case "TIME OF FIRST OBS":
			if sys := readTimeSystem(content); sys != "" {
				h.timeSystem = sys
			}
		case "TIME OF LAST OBS":
			if h.timeSystem == "" {
				if sys := readTimeSystem(content); sys != "" {
					h.timeSystem = sys
				}
			}
		case "LEAP SECONDS":
			fields := strings.Fields(content)
			if len(fields) != 0 {
				n, err := strconv.ParseInt(fields[0], 10, 16)
				if err != nil {
					return h, fmt.Errorf("rinex: invalid leap seconds %q", fields[0])
				}
				v := int16(n)
				h.meta.LeapSeconds = &v
			}
		case "END OF HEADER":
			if h.timeSystem == "" {
				h.timeSystem = fileTimeSystem(h.codes)
			}
			return h, s.Err()
		}
	}
	if err := s.Err(); err != nil {
		return h, err
	}
	return h, fmt.Errorf("rinex: missing END OF HEADER")
}

func readObservationEpochs(s *bufio.Scanner, h observationHeader) ([]SignalObservation, error) {
	var obs []SignalObservation
	leapSeconds := gpsUTCSeconds(h.meta)
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line[0] != '>' {
			return nil, fmt.Errorf("rinex: expected epoch line, got %q", line)
		}
		t, n, err := parseEpochLine(line)
		if err != nil {
			return nil, err
		}
		t = fileToGPSTime(t, h.timeSystem, leapSeconds)
		for i := 0; i < n; i++ {
			if !s.Scan() {
				if err := s.Err(); err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("rinex: unexpected EOF in epoch")
			}
			line := s.Text()
			satObs, err := parseSatelliteObservationLine(t, line, h)
			if err != nil {
				return nil, err
			}
			obs = append(obs, satObs...)
		}
	}
	return obs, s.Err()
}

func parseRunDate(s string) time.Time {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return time.Time{}
	}
	t, err := time.ParseInLocation("20060102 150405", fields[0]+" "+fields[1], time.UTC)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseFloatTriple(s string) ([3]float64, bool) {
	fields := strings.Fields(s)
	var out [3]float64
	if len(fields) < 3 {
		return out, false
	}
	for i := range out {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return out, false
		}
		out[i] = v
	}
	return out, true
}

func readObsTypesHeader(content, prev string, codes map[string][]ObservationCode, counts map[string]int) (string, error) {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return prev, nil
	}
	sys := prev
	i := 0
	if len(fields[0]) == 1 && strings.Contains("GRESJCI", fields[0]) {
		sys = fields[0]
		i = 1
	}
	if sys == "" {
		return "", fmt.Errorf("rinex: observation type continuation without system")
	}
	if len(fields) > i {
		if _, err := strconv.Atoi(fields[i]); err == nil {
			counts[sys], _ = strconv.Atoi(fields[i])
			i++
		}
	}
	for ; i < len(fields); i++ {
		code := ObservationCode(fields[i])
		if len(code) != 3 {
			return sys, fmt.Errorf("rinex: invalid observation code %q", fields[i])
		}
		codes[sys] = append(codes[sys], code)
	}
	return sys, nil
}

func readGLONASSFreqHeader(content string, frq map[SatelliteID]int8) error {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return nil
	}
	i := 0
	if _, err := strconv.Atoi(fields[0]); err == nil {
		i = 1
	}
	for i+1 < len(fields) {
		sat := SatelliteID(fields[i])
		n, err := strconv.ParseInt(fields[i+1], 10, 8)
		if err != nil {
			return fmt.Errorf("rinex: invalid GLONASS frequency %q", fields[i+1])
		}
		if sat.IsValid() {
			frq[sat] = int8(n)
		}
		i += 2
	}
	return nil
}

func readTimeSystem(content string) string {
	fields := strings.Fields(content)
	if len(fields) >= 7 {
		return fields[6]
	}
	return ""
}

func fileTimeSystem(codes map[string][]ObservationCode) string {
	systems := orderedSystems(codes)
	if len(systems) == 1 {
		return systemTimeSystem(systems[0])
	}
	return "GPS"
}

func systemTimeSystem(sys string) string {
	switch sys {
	case "R":
		return "GLO"
	case "E":
		return "GAL"
	case "J":
		return "QZS"
	case "C":
		return "BDT"
	case "I":
		return "IRN"
	default:
		return "GPS"
	}
}

func gpsUTCSeconds(meta Metadata) int64 {
	if meta.LeapSeconds != nil {
		return int64(*meta.LeapSeconds)
	}
	return defaultGPSUTCSeconds
}

func fileToGPSTime(t Time, timeSystem string, leapSeconds int64) Time {
	switch timeSystem {
	case "GLO", "UTC":
		return t + Time(leapSeconds*int64(time.Second)/tickNs)
	case "BDT":
		return t + Time(bdtGPSOffsetSeconds*int64(time.Second)/tickNs)
	default:
		return t
	}
}

func parseEpochLine(line string) (Time, int, error) {
	fields := strings.Fields(line)
	if len(fields) < 9 {
		return 0, 0, fmt.Errorf("rinex: invalid epoch line %q", line)
	}
	t, err := parseRINEXTime(fields[1], fields[2], fields[3], fields[4], fields[5], fields[6])
	if err != nil {
		return 0, 0, err
	}
	n, err := strconv.Atoi(fields[8])
	if err != nil {
		return 0, 0, fmt.Errorf("rinex: invalid epoch satellite count %q", fields[8])
	}
	return t, n, nil
}

func parseRINEXTime(year, month, day, hour, minute, second string) (Time, error) {
	y, err := strconv.Atoi(year)
	if err != nil {
		return 0, fmt.Errorf("rinex: invalid year %q", year)
	}
	mo, err := strconv.Atoi(month)
	if err != nil {
		return 0, fmt.Errorf("rinex: invalid month %q", month)
	}
	d, err := strconv.Atoi(day)
	if err != nil {
		return 0, fmt.Errorf("rinex: invalid day %q", day)
	}
	h, err := strconv.Atoi(hour)
	if err != nil {
		return 0, fmt.Errorf("rinex: invalid hour %q", hour)
	}
	m, err := strconv.Atoi(minute)
	if err != nil {
		return 0, fmt.Errorf("rinex: invalid minute %q", minute)
	}
	secText, fracText, _ := strings.Cut(second, ".")
	sec, err := strconv.Atoi(secText)
	if err != nil {
		return 0, fmt.Errorf("rinex: invalid second %q", second)
	}
	fracText += "0000000"
	tick, err := strconv.ParseInt(fracText[:7], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("rinex: invalid fractional second %q", second)
	}
	tm := time.Date(y, time.Month(mo), d, h, m, sec, 0, time.UTC)
	return TimeFromCivilTime(tm) + Time(tick), nil
}

func parseSatelliteObservationLine(t Time, line string, h observationHeader) ([]SignalObservation, error) {
	line = padLine(line, 3)
	sat := SatelliteID(line[:3])
	if !sat.IsValid() {
		return nil, fmt.Errorf("rinex: invalid satellite %q", sat)
	}
	codes := h.codes[sat.System()]
	bySig := make(map[SignalID]*SignalObservation)
	order := make([]SignalID, 0)
	for i, code := range codes {
		field := rinexField(line, i)
		if strings.TrimSpace(field) == "" {
			continue
		}
		sig := SignalID(string(code[1:]))
		o := bySig[sig]
		if o == nil {
			order = append(order, sig)
			o = &SignalObservation{T: t, Sat: sat, Sig: sig}
			if v, ok := h.frq[sat]; ok {
				o.Frq = opt.Make(v)
			}
			bySig[sig] = o
		}
		addRINEXField(o, code, field)
	}
	obs := make([]SignalObservation, 0, len(order))
	for _, sig := range order {
		obs = append(obs, *bySig[sig])
	}
	return obs, nil
}

func rinexField(line string, i int) string {
	start := 3 + i*16
	if len(line) <= start {
		return strings.Repeat(" ", 16)
	}
	end := start + 16
	if len(line) < end {
		return padLine(line[start:], 16)
	}
	return line[start:end]
}

func addRINEXField(o *SignalObservation, code ObservationCode, field string) {
	val := strings.TrimSpace(field[:14])
	switch code[0] {
	case byte(TypeCode):
		if v, ok := parseObsFloat(val); ok {
			o.PR = opt.Make(v)
		}
		if v, ok := parseIndicator(field[15]); ok {
			addSSI(o, v)
		}
	case byte(TypePhase):
		if v, ok := parseObsFloat(val); ok {
			o.CP = opt.Make(v)
		}
		if v, ok := parseIndicator(field[14]); ok {
			o.LLI = opt.Make(LLI(v))
		}
		if v, ok := parseIndicator(field[15]); ok {
			addSSI(o, v)
		}
	case byte(TypeDoppler):
		if v, ok := parseObsFloat(val); ok {
			o.Do = opt.Make(v)
		}
	case byte(TypeSignalStrength):
		if v, ok := parseObsFloat(val); ok {
			o.CN0 = opt.Make(float32(v))
		}
	}
}

func addSSI(o *SignalObservation, ssi uint8) {
	if ssi == 0 || o.CN0.IsSet() {
		return
	}
	o.CN0 = opt.Make(cn0FromSSI(ssi))
}

func cn0FromSSI(ssi uint8) float32 {
	if ssi <= 1 {
		return 6
	}
	if ssi >= 9 {
		return 57
	}
	return float32(ssi*6 + 3)
}

func parseObsFloat(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func parseIndicator(b byte) (uint8, bool) {
	if b < '0' || b > '9' {
		return 0, false
	}
	return b - '0', true
}

func padLine(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
