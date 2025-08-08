package unc

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

type nativeConfigPropID string

// idUsageFlags describes how nativeConfigPropID is used in the protocolDDeN
type nativeConfigPropIDUsage int

const (
	// usageNormal means for a property XYZ
	// 1. Set using CONFIG XYZ param...
	// 2. Queried using CONFIG with no additional parameters
	// 3. The response comes back as $CONFIG,XYZ,CONFIG XYZ param...
	// 4. There will only be a single $CONFIG,XYZ,... line in the response
	usageNormal nativeConfigPropIDUsage = iota // Normal usage
	// usageMode is like MODE command
	// 1. Set use MODE param... (no CONFIG prefix)
	// 2. Queries using MODE (no CONFIG prefix)
	// 3. Response is a single #MODE line (note that uses # not $)
	usageMode
	// usageMask is like MASK command
	// 1. Set using MASK param... (no CONFIG prefix) and UNMASK param...
	// 2. Queries using MASK (no CONFIG prefix)
	// 3. Query reponse is multiple $CONFIG,MASK,MASK param... lines
	// 4. Can be multiple response lines for single MASK query
	usageMask
)

// error indicating toNative failed because the ConfigProps did not contain the necessary properties
var errMissingProp = fmt.Errorf("missing property")

// nativeConfigProp describes property of the receiver's configuration
type nativeConfigProp interface {
	// IsZero returns true if this property is not known
	IsZero() bool
	// id returns the nativeConfigPropID for this property
	id() nativeConfigPropID
	// idUsage says how the ID is used in commands/responses in the protocol
	idUsage() nativeConfigPropIDUsage
	// generate a list of lines (without line terminators) that can be sent to the receiver
	// to set this property; empty slice if no command is needed
	generateCommands(prevProp any) []string
	// updateFromCommand updates the property based on a command string understood by the receiver.
	// The command string may be a response to query or may be one of the commands from generateCommands.
	updateFromCommand(cmd string) error
	// clone returns deep copy
	clone() nativeConfigProp
	// convertToProps sets the gpsprot.ConfigProps using this native property
	convertToProps(*gpsprot.ConfigProps) error
	// updateFromProps sets this native property from gpsprot.ConfigProps
	// returns an error if the property cannot be set
	updateFromProps(*gpsprot.ConfigProps) error
}

type nativeConfigProps map[nativeConfigPropID]nativeConfigProp

const (
	idPropPPS         nativeConfigPropID = "PPS"
	idPropSignalGroup nativeConfigPropID = "SIGNALGROUP"
	idPropMask        nativeConfigPropID = "MASK"
	idPropMode        nativeConfigPropID = "MODE"
	idPropCOM1        nativeConfigPropID = "COM1"
	idPropCOM2        nativeConfigPropID = "COM2"
	idPropCOM3        nativeConfigPropID = "COM3"
)

func NewNativeConfigProps() nativeConfigProps {
	return nativeConfigProps{
		idPropPPS:         newPPSProp(),
		idPropSignalGroup: newSignalGroupProp(),
		idPropMask:        newMaskProp(),
		idPropMode:        &modeProp{},
		idPropCOM1:        newCOMProp(idPropCOM1),
		idPropCOM2:        newCOMProp(idPropCOM2),
		idPropCOM3:        newCOMProp(idPropCOM3),
	}
}

type simpleConfigProp struct {
	name    nativeConfigPropID
	regexp  *regexp.Regexp
	command string
}

func (p *simpleConfigProp) IsZero() bool {
	return p.command == ""
}

func (p *simpleConfigProp) id() nativeConfigPropID {
	return p.name
}

func (p *simpleConfigProp) idUsage() nativeConfigPropIDUsage {
	return usageNormal
}

func (p *simpleConfigProp) updateFromCommand(cmd string) error {
	if p.regexp != nil && !p.regexp.MatchString(cmd) {
		return fmt.Errorf("invalid command format: %s", cmd)
	}
	p.command = cmd
	return nil
}

func (p *simpleConfigProp) convertToProps(props *gpsprot.ConfigProps) error {
	panic("convertToProps not implemented for simpleConfigProp")
}

func (p *simpleConfigProp) updateFromProps(props *gpsprot.ConfigProps) error {
	panic("updateFromProps not implemented for simpleConfigProp")
}

func (p *simpleConfigProp) generateCommands(prev any) []string {
	prevSimple := prev.(*simpleConfigProp)
	if prevSimple.command == p.command {
		return nil
	}
	return []string{p.command}
}

var ppsRegexp = regexp.MustCompile(
	`^CONFIG PPS (?:DISABLE|(ENABLE[23]?) (GPS|BDS|GAL|GLO) (POSITIVE|NEGATIVE) ([1-9]\d{0,5}) ([1-9]\d{0,8}) (-?\d{0,6}) (-?\d{0,6}))$`)

type ppsProp struct {
	simpleConfigProp
}

func newPPSProp() *ppsProp {
	return &ppsProp{
		simpleConfigProp: simpleConfigProp{
			name:   idPropPPS,
			regexp: ppsRegexp,
		},
	}
}

func (p *ppsProp) clone() nativeConfigProp {
	copied := *p
	return &copied
}

func (p *ppsProp) convertToProps(props *gpsprot.ConfigProps) error {
	if p.command == "" {
		return nil // No command to parse
	}

	matches := p.regexp.FindStringSubmatch(p.command)
	if matches == nil {
		return fmt.Errorf("invalid PPS command format: %s", p.command)
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
		return nil
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

	return nil
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
	if p.command != "" && p.regexp != nil {
		matches := p.regexp.FindStringSubmatch(p.command)
		if len(matches) > 7 { // ENABLE case with all capture groups
			userDelay = matches[7]
		}
	}

	// Construct the command
	p.command = fmt.Sprintf("CONFIG PPS %s %s %s %d %d %d %s",
		enable, gnssStr, polarity, widthUs, periodMs, rfDelayNs, userDelay)

	return nil
}

type signalGroupProp struct {
	simpleConfigProp
}

var signalGroupRegexp = regexp.MustCompile(`^CONFIG SIGNALGROUP ([1-9]\d?)(:? (\d\d?))$`)

func newSignalGroupProp() *signalGroupProp {
	return &signalGroupProp{
		simpleConfigProp: simpleConfigProp{
			name:   idPropSignalGroup,
			regexp: signalGroupRegexp,
		},
	}
}

func (p *signalGroupProp) clone() nativeConfigProp {
	cloned := *p
	return &cloned
}

type comProp struct {
	simpleConfigProp
}

func newCOMProp(id nativeConfigPropID) *comProp {
	return &comProp{
		simpleConfigProp: simpleConfigProp{
			name: id,
		},
	}
}

func (p *comProp) clone() nativeConfigProp {
	cloned := *p
	return &cloned
}

var maskRegexp = regexp.MustCompile(`^(?:MASK (\d+(?:\.\d+)?)|((MASK|UNMASK) (?:([A-Z][A-Z0-9]*)|[A-Z]+ PRN \d+)))$`)

type maskProp struct {
	elevationMask string // e.g. "5" for 5 degrees
	signalMask    gpsprot.SignalSet
}

func newMaskProp() *maskProp {
	return &maskProp{}
}

func (p *maskProp) clone() nativeConfigProp {
	cloned := *p
	return &cloned
}

func (p *maskProp) IsZero() bool {
	return *p == maskProp{}
}

func (p *maskProp) id() nativeConfigPropID {
	return idPropMask
}

func (p *maskProp) idUsage() nativeConfigPropIDUsage {
	return usageMask
}

func (p *maskProp) updateFromCommand(cmd string) error {
	matches := maskRegexp.FindStringSubmatch(cmd)
	if matches == nil {
		return fmt.Errorf("invalid mask command format: %s", cmd)
	}
	
	elevation := matches[1]      // numeric elevation angle (if MASK elevation)
	command := matches[3]        // MASK or UNMASK (for system/frequency cases)
	systemFreq := matches[4]     // system/frequency name (if present)
	
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

func (p *maskProp) convertToProps(props *gpsprot.ConfigProps) error {
	// TODO: implement mask to SignalsEnabled conversion
	return nil
}

func (p *maskProp) updateFromProps(props *gpsprot.ConfigProps) error {
	// TODO
	return nil
}

func (p *maskProp) generateCommands(prev any) []string {
	var commands []string
	prevMask := prev.(*maskProp)
	// Handle elevation mask changes
	if p.elevationMask != prevMask.elevationMask {
		if p.elevationMask != "" {
			commands = append(commands, "MASK "+p.elevationMask)
		}
	}
	// Handle signal mask changes
	commands = append(commands, generateMaskCommands(p.signalMask, "MASK")...)
	// Remove signals that are no longer masked
	return append(commands, generateMaskCommands(prevMask.signalMask &^ p.signalMask, "UNMASK")...)
}

// generateMaskCommands generates MASK or UNMASK commands for a given signal set
func generateMaskCommands(signals gpsprot.SignalSet, command string) []string {
	var commands []string
	targetSet := signals
	// maskSignalList is ordered with supersets first, so we will get a minimal command set
	for _, entry := range maskSignalList {
		if (entry.signalSet & targetSet) == entry.signalSet {
			commands = append(commands, command+" "+entry.name)
			targetSet &^= entry.signalSet
		}
	}
	return commands
}

type modeProp struct {
	command string
}

func (p *modeProp) IsZero() bool {
	return p.command == ""
}

func (p *modeProp) clone() nativeConfigProp {
	copied := *p
	return &copied
}

func (p *modeProp) id() nativeConfigPropID {
	return idPropMode
}

func (p *modeProp) idUsage() nativeConfigPropIDUsage {
	return usageMode
}

func (p *modeProp) updateFromCommand(cmd string) error {
	p.command = cmd
	return nil
}

func (p *modeProp) convertToProps(props *gpsprot.ConfigProps) error {
	// TODO: implement mode command parsing
	return nil
}

func (p *modeProp) updateFromProps(props *gpsprot.ConfigProps) error {
	// TODO: implement mode command serialization
	return nil
}

func (p *modeProp) generateCommands(prev any) []string {
	prevMode := prev.(*modeProp)
	if prevMode.command == p.command || p.command == "" {
		return nil
	}
	return []string{p.command}
}
