package rinex

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
)

const obsValueWidth = 14

const (
	defaultRINEXVersion = 3.04
	defaultProgram      = "ubx2rinex"
)

var systemOrder = []string{"G", "R", "E", "J", "C", "I", "S"}

type obsFile struct {
	codes  map[string][]ObservationCode
	epochs []epoch
	frq    map[SatelliteID]int8
	first  Time
	last   Time
}

type epoch struct {
	t    Time
	sats []SatelliteID
	obs  map[SatelliteID]map[ObservationCode]obsField
}

type obsField struct {
	val opt.Val[float64]
	lli opt.Val[LLI]
	ssi opt.Val[uint8]
}

// WriteObservationFile writes a RINEX observation file.
func WriteObservationFile(w io.Writer, meta Metadata, obs []SignalObservation, opts WriterOptions) error {
	f, err := buildObsFile(obs)
	if err != nil {
		return err
	}
	if opts.Version == 0 {
		opts.Version = defaultRINEXVersion
	}
	if opts.Program == "" {
		opts.Program = defaultProgram
	}
	if opts.Date.IsZero() {
		opts.Date = time.Now().UTC()
	}
	bw := bufio.NewWriter(w)
	if err := writeHeader(bw, meta, f, opts); err != nil {
		return err
	}
	if err := writeEpochs(bw, f); err != nil {
		return err
	}
	return bw.Flush()
}

func buildObsFile(obs []SignalObservation) (*obsFile, error) {
	if len(obs) == 0 {
		return nil, fmt.Errorf("rinex: no observations")
	}
	builders := make(map[Time]*epoch)
	frq := make(map[SatelliteID]int8)
	first := obs[0].T
	last := obs[0].T
	for _, o := range obs {
		if !o.Sat.IsValid() {
			return nil, fmt.Errorf("rinex: invalid satellite %q", o.Sat)
		}
		if !o.Sig.IsValid() {
			return nil, fmt.Errorf("rinex: invalid signal %q", o.Sig)
		}
		if o.T < first {
			first = o.T
		}
		if o.T > last {
			last = o.T
		}
		e := builders[o.T]
		if e == nil {
			e = &epoch{t: o.T, obs: make(map[SatelliteID]map[ObservationCode]obsField)}
			builders[o.T] = e
		}
		if e.obs[o.Sat] == nil {
			e.sats = append(e.sats, o.Sat)
			e.obs[o.Sat] = make(map[ObservationCode]obsField)
		}
		if o.Frq.IsSet() {
			frq[o.Sat] = o.Frq.Get()
		}
		addSignalObservation(e.obs[o.Sat], o)
	}
	epochs := make([]epoch, 0, len(builders))
	for _, e := range builders {
		epochs = append(epochs, *e)
	}
	slices.SortFunc(epochs, func(a, b epoch) int {
		if a.t < b.t {
			return -1
		}
		if a.t > b.t {
			return 1
		}
		return 0
	})
	return &obsFile{codes: writerObservationCodeSet(obs), epochs: epochs, frq: frq, first: first, last: last}, nil
}

func writerObservationCodeSet(obs []SignalObservation) map[string][]ObservationCode {
	seen := make(map[string]map[ObservationCode]bool)
	for _, o := range obs {
		sys := o.System()
		if sys == "" {
			continue
		}
		m := seen[sys]
		if m == nil {
			m = make(map[ObservationCode]bool)
			seen[sys] = m
		}
		for _, code := range writerObservationCodes(o) {
			if code != "" {
				m[code] = true
			}
		}
	}
	out := make(map[string][]ObservationCode, len(seen))
	for sys, m := range seen {
		codes := make([]ObservationCode, 0, len(m))
		for code := range m {
			codes = append(codes, code)
		}
		sortObservationCodes(codes)
		out[sys] = codes
	}
	return out
}

func writerObservationCodes(o SignalObservation) []ObservationCode {
	codes := make([]ObservationCode, 0, 4)
	if o.PR.IsSet() {
		codes = append(codes, o.Sig.Code(TypeCode))
	}
	if o.PR.IsSet() || o.CP.IsSet() || o.Do.IsSet() || o.CN0.IsSet() || o.LLI.IsSet() || o.SSI.IsSet() {
		codes = append(codes, o.Sig.Code(TypePhase))
	}
	if o.Do.IsSet() {
		codes = append(codes, o.Sig.Code(TypeDoppler))
	}
	if o.CN0.IsSet() {
		codes = append(codes, o.Sig.Code(TypeSignalStrength))
	}
	return codes
}

func addSignalObservation(dst map[ObservationCode]obsField, o SignalObservation) {
	if o.PR.IsSet() {
		addObsField(dst, o.Sig.Code(TypeCode), opt.Make(float64(o.PR.Get())), opt.Val[LLI]{}, opt.Val[uint8]{})
	}
	if o.CP.IsSet() {
		addObsField(dst, o.Sig.Code(TypePhase), opt.Make(o.CP.Get()), o.LLI, o.SSI)
	} else if o.LLI.IsSet() || o.SSI.IsSet() {
		addObsField(dst, o.Sig.Code(TypePhase), opt.Val[float64]{}, o.LLI, o.SSI)
	}
	if o.Do.IsSet() {
		addObsField(dst, o.Sig.Code(TypeDoppler), opt.Make(o.Do.Get()), opt.Val[LLI]{}, opt.Val[uint8]{})
	}
	if o.CN0.IsSet() {
		addObsField(dst, o.Sig.Code(TypeSignalStrength), opt.Make(float64(o.CN0.Get())), opt.Val[LLI]{}, opt.Val[uint8]{})
	}
}

func addObsField(dst map[ObservationCode]obsField, code ObservationCode, val opt.Val[float64], lli opt.Val[LLI], ssi opt.Val[uint8]) {
	if code == "" {
		return
	}
	if _, ok := dst[code]; !ok {
		dst[code] = obsField{val: val, lli: lli, ssi: ssi}
	}
}

func writeHeader(w *bufio.Writer, meta Metadata, f *obsFile, opts WriterOptions) error {
	sys := headerSystem(f.codes)
	if err := writeHeaderLine(w, fmt.Sprintf("%9.2f           OBSERVATION DATA    %-20s", opts.Version, sys), "RINEX VERSION / TYPE"); err != nil {
		return err
	}
	date := opts.Date.UTC().Format("20060102 150405 UTC")
	if err := writeHeaderLine(w, fmt.Sprintf("%-20.20s%-20.20s%-20.20s", opts.Program, opts.RunBy, date), "PGM / RUN BY / DATE"); err != nil {
		return err
	}
	for _, c := range meta.Comments {
		if err := writeHeaderLine(w, c, "COMMENT"); err != nil {
			return err
		}
	}
	if err := writeHeaderLine(w, meta.MarkerName, "MARKER NAME"); err != nil {
		return err
	}
	if err := writeHeaderLine(w, meta.MarkerNumber, "MARKER NUMBER"); err != nil {
		return err
	}
	if err := writeHeaderLine(w, meta.MarkerType, "MARKER TYPE"); err != nil {
		return err
	}
	if err := writeHeaderLine(w, fmt.Sprintf("%-20.20s%-40.40s", meta.Observer, meta.Agency), "OBSERVER / AGENCY"); err != nil {
		return err
	}
	if err := writeHeaderLine(w, fmt.Sprintf("%-20.20s%-20.20s%-20.20s", meta.Receiver.Number, meta.Receiver.Type, meta.Receiver.Version), "REC # / TYPE / VERS"); err != nil {
		return err
	}
	if err := writeHeaderLine(w, fmt.Sprintf("%-20.20s%-40.40s", meta.Antenna.Number, meta.Antenna.Type), "ANT # / TYPE"); err != nil {
		return err
	}
	pos := meta.ApproxPosition.Get()
	if err := writeHeaderLine(w, fmt.Sprintf("%14.4f%14.4f%14.4f", pos[0], pos[1], pos[2]), "APPROX POSITION XYZ"); err != nil {
		return err
	}
	delta := meta.AntennaDelta.Get()
	if err := writeHeaderLine(w, fmt.Sprintf("%14.4f%14.4f%14.4f", delta[0], delta[1], delta[2]), "ANTENNA: DELTA H/E/N"); err != nil {
		return err
	}
	for _, sys := range orderedSystems(f.codes) {
		if err := writeObsTypes(w, sys, f.codes[sys]); err != nil {
			return err
		}
	}
	if err := writeTimeHeader(w, f.first, "TIME OF FIRST OBS"); err != nil {
		return err
	}
	if err := writeTimeHeader(w, f.last, "TIME OF LAST OBS"); err != nil {
		return err
	}
	if len(f.frq) != 0 {
		if err := writeGLONASSFreq(w, f.frq); err != nil {
			return err
		}
	}
	if len(f.codes["R"]) != 0 {
		if err := writeHeaderLine(w, " C1C    0.000 C1P    0.000 C2C    0.000 C2P    0.000", "GLONASS COD/PHS/BIS"); err != nil {
			return err
		}
	}
	if meta.LeapSeconds.IsSet() {
		if err := writeHeaderLine(w, fmt.Sprintf("%6d", meta.LeapSeconds.Get()), "LEAP SECONDS"); err != nil {
			return err
		}
	}
	return writeHeaderLine(w, "", "END OF HEADER")
}

func writeHeaderLine(w *bufio.Writer, content, label string) error {
	if len(content) > 60 {
		content = content[:60]
	}
	_, err := fmt.Fprintf(w, "%-60s%-20s\n", content, label)
	return err
}

func headerSystem(codes map[string][]ObservationCode) string {
	systems := orderedSystems(codes)
	if len(systems) != 1 {
		return "M: Mixed"
	}
	switch systems[0] {
	case "G":
		return "G: GPS"
	case "R":
		return "R: GLONASS"
	case "E":
		return "E: Galileo"
	case "J":
		return "J: QZSS"
	case "C":
		return "C: BDS"
	case "I":
		return "I: NavIC"
	case "S":
		return "S: SBAS"
	}
	return systems[0]
}

func orderedSystems(codes map[string][]ObservationCode) []string {
	systems := make([]string, 0, len(codes))
	for _, sys := range systemOrder {
		if len(codes[sys]) != 0 {
			systems = append(systems, sys)
		}
	}
	for sys := range codes {
		if len(codes[sys]) != 0 && !slices.Contains(systems, sys) {
			systems = append(systems, sys)
		}
	}
	return systems
}

func writeObsTypes(w *bufio.Writer, sys string, codes []ObservationCode) error {
	for i := 0; i < len(codes); i += 13 {
		content := fmt.Sprintf("%s%5d", sys, len(codes))
		if i > 0 {
			content = sys + "     "
		}
		end := min(i+13, len(codes))
		for _, code := range codes[i:end] {
			content += fmt.Sprintf(" %-3s", code)
		}
		if err := writeHeaderLine(w, content, "SYS / # / OBS TYPES"); err != nil {
			return err
		}
	}
	return nil
}

func writeTimeHeader(w *bufio.Writer, t Time, label string) error {
	tm := t.UTC()
	sec := float64(tm.Second()) + float64(tm.Nanosecond())/1e9
	return writeHeaderLine(w, fmt.Sprintf("%6d%6.2d%6.2d%6.2d%6.2d%13s     GPS", tm.Year(), tm.Month(), tm.Day(), tm.Hour(), tm.Minute(), secondField(sec)), label)
}

func writeGLONASSFreq(w *bufio.Writer, frq map[SatelliteID]int8) error {
	sats := make([]SatelliteID, 0, len(frq))
	for sat := range frq {
		if sat.System() == "R" {
			sats = append(sats, sat)
		}
	}
	slices.Sort(sats)
	for i := 0; i < len(sats); i += 8 {
		content := fmt.Sprintf("%3d", len(sats))
		if i > 0 {
			content = "   "
		}
		end := min(i+8, len(sats))
		for _, sat := range sats[i:end] {
			content += fmt.Sprintf(" %-3s%3d", sat, frq[sat])
		}
		if err := writeHeaderLine(w, content, "GLONASS SLOT / FRQ #"); err != nil {
			return err
		}
	}
	return nil
}

func writeEpochs(w *bufio.Writer, f *obsFile) error {
	line := make([]byte, 0, 256)
	for _, e := range f.epochs {
		tm := e.t.UTC()
		sec := float64(tm.Second()) + float64(tm.Nanosecond())/1e9
		line = line[:0]
		line = fmt.Appendf(line, "> %04d %02d %02d %02d %02d %s  0%3d", tm.Year(), tm.Month(), tm.Day(), tm.Hour(), tm.Minute(), secondField(sec), len(e.sats))
		for len(line) < 56 {
			line = append(line, ' ')
		}
		line = append(line, '\n')
		if _, err := w.Write(line); err != nil {
			return err
		}
		for _, sat := range e.sats {
			line = line[:0]
			line = append(line, string(sat)...)
			codes := f.codes[sat.System()]
			fields := e.obs[sat]
			for _, code := range codes {
				field, ok := fields[code]
				if !ok {
					line = append(line, "                "...)
					continue
				}
				line = appendObsField(line, field)
			}
			line = append(line, '\n')
			if _, err := w.Write(line); err != nil {
				return err
			}
		}
	}
	return nil
}

func secondField(sec float64) string {
	return fmt.Sprintf("%010.7f", sec)
}

func appendObsField(line []byte, field obsField) []byte {
	lli := byte(' ')
	ssi := byte(' ')
	if field.lli.IsSet() {
		lli = '0' + uint8(field.lli.Get())
	}
	if field.ssi.IsSet() {
		ssi = '0' + field.ssi.Get()
	}
	var tmp [32]byte
	text := tmp[:0]
	if field.val.IsSet() {
		text = strconv.AppendFloat(text, field.val.Get(), 'f', 3, 64)
	}
	for i := len(text); i < obsValueWidth; i++ {
		line = append(line, ' ')
	}
	line = append(line, text...)
	return append(line, lli, ssi)
}
