package pmc

// This file knows about the specific managementIds we support.

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	MIDNullPTPManagement     MgmtID = 0x0000
	MIDDefaultDataSet        MgmtID = 0x2000
	MIDCurrentDataSet        MgmtID = 0x2001
	MIDParentDataSet         MgmtID = 0x2002
	MIDTimePropertiesDataSet MgmtID = 0x2003
	MIDUTCProperties         MgmtID = 0x2011
	MIDGrandmasterSettings   MgmtID = 0xC001 // GRANDMASTER_SETTINGS_NP of LinuxPTP
)

func (mid MgmtID) String() string {
	// return the same strings as in the standard
	switch mid {
	case MIDNullPTPManagement:
		return "NULL_PTP_MANAGEMENT"
	case MIDDefaultDataSet:
		return "DEFAULT_DATA_SET"
	case MIDCurrentDataSet:
		return "CURRENT_DATA_SET"
	case MIDParentDataSet:
		return "PARENT_DATA_SET"
	case MIDTimePropertiesDataSet:
		return "TIME_PROPERTIES_DATA_SET"
	case MIDUTCProperties:
		return "UTC_PROPERTIES"
	case MIDGrandmasterSettings:
		return "GRANDMASTER_SETTINGS_NP"
	}
	return fmt.Sprintf("0x%04x", uint16(mid))
}

func ParseMID(s string) (MgmtID, error) {
	switch strings.ToUpper(strings.ReplaceAll(s, "-", "_")) {
	case "NULL_PTP_MANAGEMENT":
		return MIDNullPTPManagement, nil
	case "DEFAULT_DATA_SET":
		return MIDDefaultDataSet, nil
	case "CURRENT_DATA_SET":
		return MIDCurrentDataSet, nil
	case "PARENT_DATA_SET":
		return MIDParentDataSet, nil
	case "TIME_PROPERTIES_DATA_SET":
		return MIDTimePropertiesDataSet, nil
	case "UTC_PROPERTIES":
		return MIDUTCProperties, nil
	case "GRANDMASTER_SETTINGS_NP":
		return MIDGrandmasterSettings, nil
	}
	mid, err := strconv.ParseUint(s, 0, 16)
	if err == nil {
		return MgmtID(mid), nil
	}
	return 0, fmt.Errorf("invalid management ID: %s", s)
}

func unmarshalMID(r io.Reader, mid MgmtID) (MgmtMsg, error) {
	switch mid {
	case MIDNullPTPManagement:
		return unmarshalMgmtValue[NullPTPMgmt](r)
	case MIDGrandmasterSettings:
		return unmarshalMgmtValue[GrandmasterSettings](r)
	case MIDDefaultDataSet:
		return unmarshalMgmtValue[DefaultDS](r)
	case MIDCurrentDataSet:
		return unmarshalMgmtValue[CurrentDS](r)
	case MIDParentDataSet:
		return unmarshalMgmtValue[ParentDS](r)
	case MIDTimePropertiesDataSet:
		return unmarshalMgmtValue[TimePropertiesDS](r)
	case MIDUTCProperties:
		return unmarshalMgmtValue[UTCProperties](r)
	default:
		return nil, fmt.Errorf("unsupported management ID: %v", mid)
	}
}

type NullPTPMgmtMsg = MgmtMsgWithValue[MgmtValue[NullPTPMgmt]]

type NullPTPMgmt struct{}

func (NullPTPMgmt) MgmtID() MgmtID {
	return MIDNullPTPManagement
}

type DefaultDS struct {
	Flags         DefaultDSFlags
	_             uint8
	NumberPorts   uint16
	Priority1     uint8
	ClockQuality  ClockQuality
	Priority2     uint8
	ClockIdentity ClockIdentity
	DomainNumber  uint8
	_             uint8
}

type DefaultDSFlags uint8

const (
	DefaultDSFlagTSC DefaultDSFlags = 1 << iota
	DefaultDSFlagSO
)

func (DefaultDS) MgmtID() MgmtID {
	return MIDDefaultDataSet
}

func (ds DefaultDS) TwoStepFlag() bool {
	return ds.Flags&DefaultDSFlagTSC != 0
}

func (ds DefaultDS) SlaveOnly() bool {
	return ds.Flags&DefaultDSFlagSO != 0
}

type CurrentDS struct {
	StepsRemoved     uint16
	OffsetFromMaster TimeInterval
	MeanPathDelay    TimeInterval
}

func (CurrentDS) MgmtID() MgmtID {
	return MIDCurrentDataSet
}

type ParentDS struct {
	ParentPortIdentity                    PortIdentity
	ParentStats                           bool
	_                                     uint8
	ObservedParentOffsetScaledLogVariance uint16
	ObservedParentClockPhaseChangeRate    int32
	GrandmasterPriority1                  uint8
	GrandmasterClockQuality               ClockQuality
	GrandmasterPriority2                  uint8
	GrandmasterIdentity                   ClockIdentity
}

func (ParentDS) MgmtID() MgmtID {
	return MIDParentDataSet
}

type TimePropertiesDS struct {
	CurrentUTCOffset int16
	Flags            TimePropertiesDSFlags
	TimeSource       uint8
}

type TimePropertiesDSFlags uint8

const (
	TimePropertiesDSFlagLI61 TimePropertiesDSFlags = iota
	TimePropertiesDSFlagLI59
	TimePropertiesDSFlagUTCV
	TimePropertiesDSFlagPTP
	TimePropertiesDSFlagTTRA
	TimePropertiesDSFlagFTRA
)

func (TimePropertiesDS) MgmtID() MgmtID {
	return MIDTimePropertiesDataSet
}

// Same as high bits of FlagField in the header
type TimeFlags uint8

const (
	Leap61 TimeFlags = 1 << iota
	Leap59
	CurrentUTCOffsetValid
	PTPTimescale
	TimeTraceable
	FrequencyTraceable
	SynchronizationUncertain
)

type GrandmasterSettingsMsg = MgmtMsgWithValue[MgmtValue[GrandmasterSettings]]

type GrandmasterSettings struct {
	ClockQuality ClockQuality
	UTCOffset    int16
	TimeFlags    TimeFlags
	TimeSource   TimeSource
}

type TimeSource uint8

const (
	TimeSourceAtomicClock        TimeSource = 0x10
	TimeSourceGNSS               TimeSource = 0x20
	TimeSourceTerrestrialRadio   TimeSource = 0x30
	TimeSourceSerialTimeCode     TimeSource = 0x39
	TimeSourcePTP                TimeSource = 0x40
	TimeSourceNTP                TimeSource = 0x50
	TimeSourceHandSet            TimeSource = 0x60
	TimeSourceOther              TimeSource = 0x90
	TimeSourceInternalOscillator TimeSource = 0xA0
)

func (GrandmasterSettings) MgmtID() MgmtID {
	return MIDGrandmasterSettings
}

func GrandmasterSettingsBinaryMsg(c *MsgPreparer, data GrandmasterSettings) ([]byte, error) {
	return MgmtSetBinaryMsg(c, data)
}

type UTCPropertiesMsg = MgmtMsgWithValue[MgmtValue[UTCProperties]]

type UTCProperties struct {
	CurrentUTCOffset int16
	TimeFlags        TimeFlags // Only Leap59, Leap61 and CurrentUTCOffsetValid are valid here
	_                uint8
}

func (UTCProperties) MgmtID() MgmtID {
	return MIDUTCProperties
}
