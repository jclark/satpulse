package unc

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

const (
	// idPropPPS works like this
	// 1. Set using CONFIG PPS param...
	// 2. Queried using CONFIG with no additional parameters
	// 3. The response comes back as $CONFIG,PPS,CONFIG PPS param...
	// 4. There will only be a single $CONFIG,PPS,... line in the response
	idPropPPS = "PPS"
	// idPropSignalGroup  works like PPS
	idPropSignalGroup = "SIGNALGROUP"
	// idPropMask works like this
	// 1. Set using MASK param... (no CONFIG prefix) and UNMASK param...
	// 2. Queries using MASK (no CONFIG prefix)
	// 3. Query reponse is multiple $CONFIG,MASK,MASK param... lines
	// 4. Can be multiple response lines for single MASK query
	idPropMask = "MASK"
	// idPropMode works like this
	// 1. Set use MODE param... (no CONFIG prefix)
	// 2. Queries using MODE (no CONFIG prefix)
	// 3. Response is a single #MODE line (note that uses # not $)
	idPropMode = "MODE"
)

// nativeConfigProp describes property of the receiver's configuration
type nativeConfigProp interface {
	// updateFromCommand updates the property based on a command string understood by the receiver.
	// The command string may be a response to query or may be one of the commands from generateCommands.
	updateFromCommand(cmd string) error
}

type nativeConfigProps struct {
	pps         ppsProp
	signalGroup signalGroupProp
	mask        maskProp
	mode        modeProp
}

func (np *nativeConfigProps) getProp(propID string) nativeConfigProp {
	switch propID {
	case idPropPPS:
		return &np.pps
	case idPropSignalGroup:
		return &np.signalGroup
	case idPropMask:
		return &np.mask
	case idPropMode:
		return &np.mode
	}
	return nil
}

func makeNativeProps() nativeConfigProps {
	return nativeConfigProps{}
}

func generateQueryCommands(_ *gpsprot.ConfigTarget) []string {
	return []string{
		"CONFIG",
		"MODE",
		"MASK",
	}
}

// error indicating toNative failed because the ConfigProps did not contain the necessary properties
var errMissingProp = fmt.Errorf("missing property")

// generateConfigCommands generates commands to change the native state to have the abstract properties specified by target
func (np *nativeConfigProps) generateConfigCommands(target *gpsprot.ConfigTarget) []string {
	// Create the full set of target abstract props
	// Convert currrent native props to abstract props
	fullTargetProps := gpsprot.ConfigProps{}
	np.convertToProps(&fullTargetProps)
	// Combine the target abstract props with partial abstract props
	// to get complete set of target abstract props
	fullTargetProps.CopyFrom(&target.Props)

	// Handle SetStatic by forcing Mode.Static = true
	if target.Opts.SetStatic {
		if mode, modeSet := fullTargetProps.GetMode(); modeSet {
			mode.Static = true
			fullTargetProps.SetMode(mode)
		} else {
			// SetStatic without existing mode → create static mode
			fullTargetProps.SetMode(gpsprot.Mode{Static: true})
		}
	}

	// Make a copy of the native props, which will become the target
	targetNativeProps := *np
	// map the complete target to the native props
	targetNativeProps.updateFromProps(&fullTargetProps, target.Opts.Survey)
	// generate commands to turn current native props to target native props
	return targetNativeProps.generateCommands(np)
}

func (np *nativeConfigProps) convertToProps(props *gpsprot.ConfigProps) {
	// Convert all native properties to gpsprot.ConfigProps
	np.pps.convertToProps(props)
	sigs := np.signalGroup.signalSet()
	if sigs != 0 {
		sigs &^= np.mask.signalMask
		props.SetSignalsEnabled(sigs)
	}
	np.mode.convertToProps(props)
}

func (np *nativeConfigProps) generateCommands(prevProps *nativeConfigProps) []string {
	cmds := np.pps.generateCommands(&prevProps.pps)
	// This won't do anything because we are not changing signal group.
	cmds = append(cmds, np.signalGroup.generateCommands(&prevProps.signalGroup)...)
	cmds = append(cmds, np.mask.generateCommands(&prevProps.mask, np.signalGroup.signalSet())...)
	cmds = append(cmds, np.mode.generateCommands(&prevProps.mode)...)
	return cmds
}

func (np *nativeConfigProps) updateFromQueryResponse(key, cmd string) error {
	prop := np.getProp(key)
	if prop == nil {
		return nil
	}
	return prop.updateFromCommand(cmd)
}

// updateFromProps updates all native properties from ConfigProps and Survey
func (np *nativeConfigProps) updateFromProps(props *gpsprot.ConfigProps, survey gpsprot.Survey) error {
	err := np.pps.updateFromProps(props)
	if err != nil && err != errMissingProp {
		return err
	}
	err = np.mode.updateFromProps(props, survey)
	if err != nil && err != errMissingProp {
		return err
	}
	availSignals := np.signalGroup.signalSet()
	if availSignals != 0 {
		np.mask.updateFromProps(props, availSignals)
	}

	// TODO: Handle SBAS separately (CONFIG SBAS ENABLE/DISABLE)
	// SBAS signals are not part of SIGNALGROUP, they're enabled via CONFIG SBAS

	return nil
}

var ppsRegexp = regexp.MustCompile(
	`^CONFIG PPS (?:DISABLE|(ENABLE[23]?) (GPS|BDS|GAL|GLO) (POSITIVE|NEGATIVE) ([1-9]\d{0,5}) ([1-9]\d{0,8}) (-?\d{0,6}) (-?\d{0,6}))$`)

type ppsProp struct {
	command string
}

func (p *ppsProp) updateFromCommand(cmd string) error {
	if !ppsRegexp.MatchString(cmd) {
		return fmt.Errorf("invalid PPS command format: %s", cmd)
	}
	p.command = cmd
	return nil
}

func (p *ppsProp) generateCommands(prev *ppsProp) []string {
	if prev.command == p.command || p.command == "" {
		return nil
	}
	return []string{p.command}
}

func (p *ppsProp) convertToProps(props *gpsprot.ConfigProps) {
	if p.command == "" {
		return // No command to parse
	}

	matches := ppsRegexp.FindStringSubmatch(p.command)
	if matches == nil {
		panic("invalid PPS command format: " + p.command)
	}

	if matches[1] == "" {
		// DISABLE case - set width to 0
		timePulse := gpsprot.TimePulse{
			Width:          0, // Disabled
			Period:         time.Second,
			PolarityRising: true,
			OnlyWhenLocked: true,
			AlignToGNSS:    true,
		}
		props.SetTimePulse(timePulse)
		return
	}

	// Parse ENABLE case with parameters
	enable := matches[1]
	timeRef := matches[2]
	polarity := matches[3]
	width, _ := strconv.ParseInt(matches[4], 10, 64)  // width comes first (microseconds)
	period, _ := strconv.ParseInt(matches[5], 10, 64) // period comes second (milliseconds)
	rfDelay, _ := strconv.ParseInt(matches[6], 10, 64)

	// Set TimePulse properties
	timePulse := gpsprot.TimePulse{
		Period:         time.Duration(period) * time.Millisecond,
		Width:          time.Duration(width) * time.Microsecond,
		PolarityRising: polarity == "POSITIVE",
	}
	switch enable {
	default:
		timePulse.OnlyWhenLocked = true
		timePulse.AlignToGNSS = true
	case "ENABLE2":
		timePulse.OnlyWhenLocked = false
		timePulse.AlignToGNSS = true
	case "ENABLE3":
		timePulse.OnlyWhenLocked = false
		timePulse.AlignToGNSS = false
	}
	props.SetTimePulse(timePulse)

	gnss, err := gpsprot.ParseGNSS(timeRef)
	if err != nil {
		panic(err) // Should not happen, as regex ensures valid GNSS
	}
	props.SetTimeGNSS(gnss)
	props.SetAntennaCableDelay(time.Duration(rfDelay) * time.Nanosecond)
}

const maxPPSPeriodSecs = 20

func (p *ppsProp) updateFromProps(props *gpsprot.ConfigProps) error {
	// Need TimePulse to construct PPS command
	timePulse, ok := props.GetTimePulse()
	if !ok {
		return errMissingProp
	}

	// If width is 0, generate DISABLE command
	if timePulse.Width == 0 {
		p.command = "CONFIG PPS DISABLE"
		return nil
	}

	// For ENABLE, we also need TimeGNSS
	timeGNSS, ok := props.GetTimeGNSS()
	if !ok {
		return errMissingProp
	}

	// Get antenna cable delay (optional, defaults to 0)
	cableDelay, _ := props.GetAntennaCableDelay()
	enable := "ENABLE"

	if !timePulse.OnlyWhenLocked {
		if timePulse.AlignToGNSS {
			enable = "ENABLE2"
		} else {
			enable = "ENABLE3"
		}
	}
	// XXX OnlyWhenLocked but !AlignToGNSS does not make much sense to me

	// Convert GNSS enum to string
	gnssStr := timeGNSS.String()

	// Polarity
	polarity := "NEGATIVE"
	if timePulse.PolarityRising {
		polarity = "POSITIVE"
	}

	// Convert period to milliseconds
	periodMs := timePulse.Period.Milliseconds()
	if periodMs <= 0 || periodMs > maxPPSPeriodSecs*1000 {
		return fmt.Errorf("PPS period out of range: %v", timePulse.Period)
	}

	// Convert width to microseconds
	widthUs := timePulse.Width.Microseconds()
	if widthUs <= 0 || widthUs >= periodMs*1000 {
		return fmt.Errorf("PPS width out of range: %v", timePulse.Width)
	}

	rfDelayNs := cableDelay.Nanoseconds()
	if rfDelayNs < math.MinInt16 || rfDelayNs > math.MaxInt16 {
		return fmt.Errorf("antenna cable delay out of range: %v", cableDelay)
	}

	// userDelay preserved from existing command (native-only field)
	userDelay := "0" // default value
	if p.command != "" {
		matches := ppsRegexp.FindStringSubmatch(p.command)
		if matches[7] != "" {
			userDelay = matches[7]
		}
	}

	// Construct the command
	p.command = fmt.Sprintf("CONFIG PPS %s %s %s %d %d %d %s",
		enable, gnssStr, polarity, widthUs, periodMs, rfDelayNs, userDelay)

	return nil
}

type signalGroupProp struct {
	command string
}

var signalGroupRegexp = regexp.MustCompile(`^CONFIG SIGNALGROUP ([1-9]\d?)(:? (\d\d?))?$`)

func (p *signalGroupProp) updateFromCommand(cmd string) error {
	if !signalGroupRegexp.MatchString(cmd) {
		return fmt.Errorf("invalid CONFIG SIGNALGROUP format: %s", cmd)
	}
	p.command = cmd
	return nil
}

func (p *signalGroupProp) generateCommands(prev *signalGroupProp) []string {
	if prev.command == p.command || p.command == "" {
		return nil
	}
	return []string{p.command}
}

// signalSet returns the SignalSet for the signal group for the main antenna
// Returns 0 if no signal group is set or if the group number is invalid
func (p *signalGroupProp) signalSet() gpsprot.SignalSet {
	if p.command == "" {
		return 0
	}

	// Parse signal group number from the command
	matches := signalGroupRegexp.FindStringSubmatch(p.command)
	groupNum, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil || groupNum < 0 || int(groupNum) >= len(signalGroups) {
		return 0
	}

	return signalGroups[groupNum]
}

var maskRegexp = regexp.MustCompile(`^(?:MASK (\d+(?:\.\d+)?)|((MASK|UNMASK) (?:([A-Z][A-Z0-9]*)|[A-Z]+ PRN \d+)))$`)

type maskProp struct {
	elevationMask string // e.g. "5" for 5 degrees
	signalMask    gpsprot.SignalSet
}

func (p *maskProp) updateFromCommand(cmd string) error {
	matches := maskRegexp.FindStringSubmatch(cmd)
	if matches == nil {
		return fmt.Errorf("invalid mask command format: %s", cmd)
	}

	elevation := matches[1]  // numeric elevation angle (if MASK elevation)
	command := matches[3]    // MASK or UNMASK (for system/frequency cases)
	systemFreq := matches[4] // system/frequency name (if present)

	if elevation != "" {
		// Handle elevation mask: MASK 5.0 (only MASK allowed)
		p.elevationMask = elevation
	} else if systemFreq != "" {
		// Handle system/frequency mask: MASK GPS or UNMASK L1
		signalSet, exists := maskSignalMap[systemFreq]
		if !exists {
			return fmt.Errorf("unknown mask target: %s", systemFreq)
		}

		if command == "MASK" {
			// Add these signals to the mask (they should be disabled)
			p.signalMask = p.signalMask | signalSet
		} else { // UNMASK
			// Remove these signals from the mask (they should be enabled)
			p.signalMask = p.signalMask &^ signalSet
		}
	}
	// PRN masks are matched by the regexp but ignored (no capture group)

	return nil
}

func (p *maskProp) updateFromProps(props *gpsprot.ConfigProps, availSignals gpsprot.SignalSet) error {
	signalsEnabled, ok := props.GetSignalsEnabled()
	if !ok {
		return errMissingProp
	}
	p.signalMask = availSignals &^ signalsEnabled
	return nil
}

func (p *maskProp) generateCommands(prevMask *maskProp, sigGroupSignals gpsprot.SignalSet) []string {
	var commands []string
	// Handle elevation mask changes
	if p.elevationMask != prevMask.elevationMask {
		if p.elevationMask != "" {
			commands = append(commands, "MASK "+p.elevationMask)
		}
	}
	// Handle signal mask changes
	commands = append(commands, generateMaskCommands(p.signalMask, sigGroupSignals, "MASK")...)
	// Remove signals that are no longer masked
	return append(commands, generateMaskCommands(prevMask.signalMask&^p.signalMask, sigGroupSignals, "UNMASK")...)
}

// generateMaskCommands generates MASK or UNMASK commands for a given signal set
func generateMaskCommands(signals gpsprot.SignalSet, sigGroupSignals gpsprot.SignalSet, command string) []string {
	var commands []string
	targetSet := signals
	// maskSignalList is ordered with supersets first, so we will get a minimal command set
	for _, entry := range maskSignalList {
		if targetSet == 0 {
			break // No more signals to mask/unmask
		}
		// Using sigGroupSignals gives a slightly nicer set of MASK commands
		// For example, signal group 2 does not include QZSS L6
		// If using satpulsetool we do not include QZSS, then we want to get MASK QZSS
		// But if we use applicable signals here, we would instead mask individually Q1, Q2 and Q5.
		// This is safe since SIGNALGROUP does a reset.
		applicableSignals := entry.signalSet & sigGroupSignals
		if applicableSignals != 0 && (applicableSignals&targetSet) == applicableSignals {
			commands = append(commands, command+" "+entry.name)
			targetSet &^= entry.signalSet
		}
	}
	return commands
}

var modeRegexp = regexp.MustCompile(
	`^MODE (?:(ROVER(?: [A-Z]+)?(?: [A-Z]+)?)|(HEADING2(?: [A-Z]+)?)|(BASE)(?: (\d+))?(?: TIME (\d+)(?: (\d+))?| (-?\d+\.?\d*) (-?\d+\.?\d*) (-?\d+\.?\d*))?)$`)

type modeProp struct {
	command string
}

func (p *modeProp) updateFromCommand(cmd string) error {
	if !modeRegexp.MatchString(cmd) {
		return fmt.Errorf("invalid MODE command format: %s", cmd)
	}
	p.command = cmd
	return nil
}

func (p *modeProp) convertToProps(props *gpsprot.ConfigProps) {
	if p.command == "" {
		return
	}

	matches := modeRegexp.FindStringSubmatch(p.command)
	if matches == nil {
		// command was validated in updateFromCommand, so this shouldn't happen
		panic("invalid MODE command format: " + p.command)
	}

	baseIDMatch := matches[4] // Base ID (if present)
	// Set base ID if present (group 4)
	if baseIDMatch != "" {
		if baseID, err := strconv.ParseUint(baseIDMatch, 10, 16); err == nil {
			props.SetRTCMBaseID(uint16(baseID))
		}
	}

	// Parse the MODE command and set appropriate properties
	roverMatch := matches[1]
	heading2Match := matches[2]

	if roverMatch != "" || heading2Match != "" {
		// ROVER or HEADING2 mode - not static
		props.SetMode(gpsprot.Mode{Static: false})
		return
	}

	// regexp guarantees that we matched BASE
	mode := gpsprot.Mode{
		Static:  true,
		PosType: gpsprot.PosTypeNone,
	}

	const coord0 = 7
	if matches[coord0] == "" {
		props.SetMode(mode) // No coordinates provided, just base ID
		return
	}

	var coords [3]float64

	for i := range 3 {
		var err error
		coordMatch := matches[coord0+i]
		coords[i], err = strconv.ParseFloat(coordMatch, 64)
		if err != nil {
			// regexp should have caught this
			panic("invalid coordinate: " + coordMatch)
		}
	}
	if coordsIsLLH(coords) {
		mode.PosType = gpsprot.PosTypeLLH
		mode.FixedPosLLH[0] = gpsprot.DegreesFromFloat(coords[0])
		mode.FixedPosLLH[1] = gpsprot.DegreesFromFloat(coords[1])
		mode.Height = gpsprot.Meters(coords[2])
	} else if coordsIsECEF(coords) {
		mode.PosType = gpsprot.PosTypeECEF
		for i := range 3 {
			mode.FixedPosECEF[i] = gpsprot.Meters(coords[i])
		}
	}

	// XXX need to decide what to do about FixedPosAcc
	props.SetMode(mode)
}

func coordsIsLLH(coords [3]float64) bool {
	for i, coord := range coords {
		if !coordIsLLH(i, coord) {
			return false
		}
	}
	return true
}

func coordsIsECEF(coords [3]float64) bool {
	for i, coord := range coords {
		if coordIsLLH(i, coord) {
			return false
		}
	}
	return true
}

var maxCoordsLLH = [3]float64{90, 180, 30000}

func coordIsLLH(i int, coord float64) bool {
	return -maxCoordsLLH[i] <= coord && coord <= maxCoordsLLH[i]
}

func (p *modeProp) updateFromProps(props *gpsprot.ConfigProps, survey gpsprot.Survey) error {
	m, modeSet := props.GetMode()

	// Only generate a command if mode is explicitly set
	if !modeSet {
		// No mode configuration requested
		return errMissingProp
	}
	var matches []string
	if p.command != "" {
		matches = modeRegexp.FindStringSubmatch(p.command)
		if matches == nil {
			panic("invalid MODE command format: " + p.command)
		}
	}
	if !m.Static {
		// if already ROVER or HEADING2 mode then leave as is, so as to preserve existing qualifiers;
		// otherwise use default ROVER mode
		if matches == nil || (matches[1] == "" && matches[2] == "") {
			p.command = "MODE ROVER"
		}
		return nil
	}
	// Get RTCM base ID, use invalid marker if not set
	cmd := "MODE BASE"
	if baseID, ok := props.GetRTCMBaseID(); ok {
		cmd += fmt.Sprintf(" %d", baseID)
	}
	switch m.PosType {
	case gpsprot.PosTypeLLH:
		// LLH coordinates: lat/lon 11 decimal places, altitude 6 decimal places
		cmd += fmt.Sprintf(" %.11f %.11f %.6f", m.FixedPosLLH[0].Degrees(), m.FixedPosLLH[1].Degrees(), m.Height.Meters())
	case gpsprot.PosTypeECEF:
		// ECEF coordinates: all 4 decimal places
		cmd += fmt.Sprintf(" %.4f %.4f %.4f", m.FixedPosECEF[0].Meters(), m.FixedPosECEF[1].Meters(), m.FixedPosECEF[2].Meters())
	case gpsprot.PosTypeNone:
		surveySecs := int64(survey.MinDur.Round(time.Second) / time.Second)
		cmd += fmt.Sprintf(" TIME %d", surveySecs)
		// Preserve TIME distance parameter if present in existing command
		if matches != nil && matches[6] != "" {
			cmd += fmt.Sprintf(" %s", matches[6])
		}
	}
	p.command = cmd
	return nil
}

func (p *modeProp) generateCommands(prevMode *modeProp) []string {
	if prevMode.command == p.command || p.command == "" {
		return nil
	}
	return []string{p.command}
}
