package unc

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

type nativeConfigPropID string

// idUsageFlags describes how nativeConfigPropID is used
type nativeConfigPropIDUsage int

// usageNormal means for a property XYZ
// 1. It is set using CONFIG XYZ param...
// 2. It queried using CONFIG with no additional parameters
// 3. The response comes back as $CONFIG,XYZ,CONFIG XYZ param...
// 4. There will only be a single $CONFIG,XYZ,... line in the response
const (
	// usageNormal means for a property XYZ
	// 1. It is set using CONFIG XYZ param...
	// 2. It queried using CONFIG with no additional parameters
	// 3. The response comes back as $CONFIG,XYZ,CONFIG XYZ param...
	// 4. There will only be a single $CONFIG,XYZ,... line in the response
	usageNormal nativeConfigPropIDUsage = iota // Normal usage
	// usageMode is like MODE command
	// 1. Set use MODE param... (no CONFIG prefix)
	// 2. Queries using MODE (no CONFIG prefix)
	// 3. Response if a single #MODE line (note # not $)
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
	// update updates the property based on a command string received from the receiver.
	// The command string may be a response to query or may be one of the commands from generateCommands.
	update(cmd string) error
	// return new zero instance
	newInstance() nativeConfigProp
	// fromNative sets the gpsprot.ConfigProps using this native property
	fromNative(*gpsprot.ConfigProps) error
	// toNative sets this native property from gpsprot.ConfigProps
	// returns an error if the property cannot be set
	toNative(*gpsprot.ConfigProps) error
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

func (p *simpleConfigProp) update(cmd string) error {
	if p.regexp != nil && !p.regexp.MatchString(cmd) {
		return fmt.Errorf("invalid command format: %s", cmd)
	}
	p.command = cmd
	return nil
}

func (p *simpleConfigProp) fromNative(props *gpsprot.ConfigProps) error {
	panic("fromNative not implemented for simpleConfigProp")
}

func (p *simpleConfigProp) toNative(props *gpsprot.ConfigProps) error {
	panic("toNative not implemented for simpleConfigProp")
}

func (p *simpleConfigProp) generateCommands(prev any) []string {
	prevSimple := prev.(*simpleConfigProp)
	if prevSimple.command == p.command {
		return nil
	}
	return []string{p.command}
}

var ppsRegexp = `^CONFIG PPS (?:DISABLE|(ENABLE[23]?) (GPS|BDS|GAL|GLO) (POSITIVE|NEGATIVE) ([1-9]\d{0,5}) ([1-9]\d{0,8}) (-?\d{0,6}) (-?\d{0,6}))$`

type ppsProp struct {
	simpleConfigProp
}

func newPPSProp() *ppsProp {
	return &ppsProp{
		simpleConfigProp: simpleConfigProp{
			name:   idPropPPS,
			regexp: regexp.MustCompile(ppsRegexp),
		},
	}
}

func (p *ppsProp) newInstance() nativeConfigProp {
	return newPPSProp()
}

func (p *ppsProp) fromNative(props *gpsprot.ConfigProps) error {
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

func (p *ppsProp) toNative(props *gpsprot.ConfigProps) error {
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

	// userDelay is always 0 for our purposes
	userDelay := 0

	// Construct the command
	p.command = fmt.Sprintf("CONFIG PPS %s %s %s %d %d %d %d",
		enable, gnssStr, polarity, widthUs, periodMs, rfDelayNs, userDelay)

	return nil
}

type signalGroupProp struct {
	simpleConfigProp
}

const signalGroupRegexp = `^CONFIG SIGNALGROUP ([1-9]\d?)(:? (\d\d?))$`

func newSignalGroupProp() *signalGroupProp {
	return &signalGroupProp{
		simpleConfigProp: simpleConfigProp{
			name:   idPropSignalGroup,
			regexp: regexp.MustCompile(signalGroupRegexp),
		},
	}
}

func (p *signalGroupProp) newInstance() nativeConfigProp {
	return newSignalGroupProp()
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

func (p *comProp) newInstance() nativeConfigProp {
	return newCOMProp(p.name)
}

type maskProp struct {
	masks map[string]struct{}
}

func newMaskProp() *maskProp {
	return &maskProp{
		masks: make(map[string]struct{}),
	}
}

func (p *maskProp) newInstance() nativeConfigProp {
	return newMaskProp()
}

func (p *maskProp) IsZero() bool {
	return len(p.masks) == 0
}

func (p *maskProp) id() nativeConfigPropID {
	return idPropMask
}

func (p *maskProp) idUsage() nativeConfigPropIDUsage {
	return usageMask
}

func (p *maskProp) update(cmd string) error {
	args := strings.Split(cmd, ",")
	if len(args) != 2 || args[0] != "MASK" {
		return nil
	}
	p.masks[args[1]] = struct{}{}
	return nil
}

func (p *maskProp) fromNative(props *gpsprot.ConfigProps) error {
	// TODO: implement mask to SignalsEnabled conversion
	return nil
}

func (p *maskProp) toNative(props *gpsprot.ConfigProps) error {
	// TODO
	return nil
}

func (p *maskProp) generateCommands(prev any) []string {
	// TODO: generate MASK/UNMASK commands
	return []string{}
}

type modeProp struct {
	command string
}

func (p *modeProp) IsZero() bool {
	return p.command == ""
}

func (p *modeProp) newInstance() nativeConfigProp {
	return &modeProp{}
}

func (p *modeProp) id() nativeConfigPropID {
	return idPropMode
}

func (p *modeProp) idUsage() nativeConfigPropIDUsage {
	return usageMode
}

func (p *modeProp) update(cmd string) error {
	p.command = cmd
	return nil
}

func (p *modeProp) fromNative(props *gpsprot.ConfigProps) error {
	// TODO: implement mode command parsing
	return nil
}

func (p *modeProp) toNative(props *gpsprot.ConfigProps) error {
	// TODO: implement mode command serialization
	return nil
}

func (p *modeProp) generateCommands(prev any) []string {
	prevMode := prev.(*modeProp)
	if prevMode.command == p.command {
		return nil
	}
	if p.command == "" {
		return []string{"MODE,0"}
	}
	return []string{p.command}
}
