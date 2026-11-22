package rinex

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/ascii"
	"github.com/jclark/satpulse/gps/lib/opt"
)

type observationHeader struct {
	meta       Metadata
	codes      map[string][]ObservationCode
	frq        map[SatelliteID]int8
	sys        string
	timeSystem string
}

type epochLine struct {
	t     Time
	flag  int
	count int
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
		case "SYS / SCALE FACTOR":
			return h, fmt.Errorf("rinex: unsupported SYS / SCALE FACTOR header")
		case "SIGNAL STRENGTH UNIT":
			if err := readSignalStrengthUnit(content); err != nil {
				return h, err
			}
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
	arc := make(map[signalKey]uint32)
	leapSeconds := gpsUTCSeconds(h.meta)
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line[0] != '>' {
			return nil, fmt.Errorf("rinex: expected epoch line, got %q", line)
		}
		e, err := parseEpochLine(line)
		if err != nil {
			return nil, err
		}
		switch e.flag {
		case 0, 1:
		case 2, 3, 5:
			if err := skipEpochRecords(s, e.count, "event records"); err != nil {
				return nil, err
			}
			continue
		case 4:
			return nil, fmt.Errorf("rinex: unsupported mid-file header update")
		case 6:
			if err := skipEpochRecords(s, e.count, "cycle-slip records"); err != nil {
				return nil, err
			}
			continue
		default:
			return nil, fmt.Errorf("rinex: unsupported epoch flag %d", e.flag)
		}
		t := fileToGPSTime(e.t, h.timeSystem, leapSeconds)
		for i := 0; i < e.count; i++ {
			if !s.Scan() {
				if err := s.Err(); err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("rinex: unexpected EOF in epoch")
			}
			line := s.Text()
			satObs, err := parseSatelliteObservationLine(t, line, h, arc)
			if err != nil {
				return nil, err
			}
			obs = append(obs, satObs...)
		}
	}
	return obs, s.Err()
}

func skipEpochRecords(s *bufio.Scanner, n int, context string) error {
	for range n {
		if !s.Scan() {
			if err := s.Err(); err != nil {
				return err
			}
			return fmt.Errorf("rinex: unexpected EOF in %s", context)
		}
	}
	return nil
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

func readSignalStrengthUnit(content string) error {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return fmt.Errorf("rinex: missing signal strength unit")
	}
	unit := strings.ToUpper(fields[0])
	if unit == "DBHZ" || unit == "DB-HZ" {
		return nil
	}
	return fmt.Errorf("rinex: unsupported signal strength unit %q", fields[0])
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

func parseEpochLine(line string) (epochLine, error) {
	line = padLine(line, 35)
	flag, err := parseEpochInt(line[31:32], "flag")
	if err != nil {
		return epochLine{}, err
	}
	n, err := parseEpochInt(line[32:35], "record count")
	if err != nil {
		return epochLine{}, err
	}
	if n < 0 {
		return epochLine{}, fmt.Errorf("rinex: invalid epoch record count %q", strings.TrimSpace(line[32:35]))
	}
	e := epochLine{flag: flag, count: n}
	if flag == 0 || flag == 1 {
		t, err := parseRINEXTime(strings.TrimSpace(line[2:6]), strings.TrimSpace(line[7:9]), strings.TrimSpace(line[10:12]), strings.TrimSpace(line[13:15]), strings.TrimSpace(line[16:18]), strings.TrimSpace(line[19:29]))
		if err != nil {
			return epochLine{}, err
		}
		e.t = t
	}
	return e, nil
}

func parseEpochInt(s, name string) (int, error) {
	text := strings.TrimSpace(s)
	if text == "" {
		return 0, fmt.Errorf("rinex: missing epoch %s", name)
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("rinex: invalid epoch %s %q", name, text)
	}
	return n, nil
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

func parseSatelliteObservationLine(t Time, line string, h observationHeader, arc map[signalKey]uint32) ([]SignalObservation, error) {
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
			o.Arc = arc[signalKey{sat: sat, sig: sig}]
			if v, ok := h.frq[sat]; ok {
				o.Frq = opt.Make(v)
			}
			bySig[sig] = o
		}
		addRINEXField(o, code, field, arc)
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

func addRINEXField(o *SignalObservation, code ObservationCode, field string, arc map[signalKey]uint32) {
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
			lli := lossOfLockIndicator(v)
			k := signalKey{sat: o.Sat, sig: o.Sig}
			if lli&lossOfLockIndicatorLostLock != 0 {
				arc[k]++
			}
			o.setRINEXLLI(lli, arc[k])
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

// parseIndicator parses a RINEX LLI/SSI indicator, a single decimal digit.
func parseIndicator(b byte) (uint8, bool) {
	return ascii.DigitVal(b)
}

func padLine(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
