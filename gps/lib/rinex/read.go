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
	meta  Metadata
	codes map[string][]ObservationCode
	frq   map[SatelliteID]int8
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
		case "COMMENT":
			h.meta.Comments = append(h.meta.Comments, strings.TrimRight(content, " "))
		case "MARKER NAME":
			h.meta.MarkerName = strings.TrimSpace(content)
		case "MARKER NUMBER":
			h.meta.MarkerNumber = strings.TrimSpace(content)
		case "MARKER TYPE":
			h.meta.MarkerType = strings.TrimSpace(content)
		case "OBSERVER / AGENCY":
			h.meta.Observer = strings.TrimSpace(content[:20])
			h.meta.Agency = strings.TrimSpace(content[20:])
		case "REC # / TYPE / VERS":
			h.meta.Receiver.Number = strings.TrimSpace(content[:20])
			h.meta.Receiver.Type = strings.TrimSpace(content[20:40])
			h.meta.Receiver.Version = strings.TrimSpace(content[40:])
		case "ANT # / TYPE":
			h.meta.Antenna.Number = strings.TrimSpace(content[:20])
			h.meta.Antenna.Type = strings.TrimSpace(content[20:])
		case "APPROX POSITION XYZ":
			if v, ok := parseFloatTriple(content); ok {
				h.meta.ApproxPosition = opt.Make(v)
			}
		case "ANTENNA: DELTA H/E/N":
			if v, ok := parseFloatTriple(content); ok {
				h.meta.AntennaDelta = opt.Make(v)
			}
		case "SYS / # / OBS TYPES":
			if err := readObsTypesHeader(content, h.codes, obsTypeCount); err != nil {
				return h, err
			}
		case "GLONASS SLOT / FRQ #":
			if err := readGLONASSFreqHeader(content, h.frq); err != nil {
				return h, err
			}
		case "LEAP SECONDS":
			fields := strings.Fields(content)
			if len(fields) != 0 {
				n, err := strconv.ParseInt(fields[0], 10, 16)
				if err != nil {
					return h, fmt.Errorf("rinex: invalid leap seconds %q", fields[0])
				}
				h.meta.LeapSeconds = opt.Make(int16(n))
			}
		case "END OF HEADER":
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

func readObsTypesHeader(content string, codes map[string][]ObservationCode, counts map[string]int) error {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return nil
	}
	sys := fields[0]
	i := 1
	if len(fields) > 1 {
		if _, err := strconv.Atoi(fields[i]); err == nil {
			counts[sys], _ = strconv.Atoi(fields[i])
			i++
		}
	}
	for ; i < len(fields); i++ {
		code := ObservationCode(fields[i])
		if len(code) != 3 {
			return fmt.Errorf("rinex: invalid observation code %q", fields[i])
		}
		codes[sys] = append(codes[sys], code)
	}
	return nil
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
	return TimeFromUTC(tm) + Time(tick), nil
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
	case byte(TypePhase):
		if v, ok := parseObsFloat(val); ok {
			o.CP = opt.Make(v)
		}
		if v, ok := parseIndicator(field[14]); ok {
			o.LLI = opt.Make(v)
		}
		if v, ok := parseIndicator(field[15]); ok {
			o.SSI = opt.Make(v)
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
