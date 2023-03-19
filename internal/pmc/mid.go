package pmc

// This file knows about the specific managementIds we support.

import (
	"fmt"
	"io"
)

const (
	MIDNullPTPManagement     MgmtID = 0x0000
	MIDDefaultDataSet        MgmtID = 0x2000
	MIDCurrentDataSet        MgmtID = 0x2001
	MIDParentDataSet         MgmtID = 0x2002
	MIDTimePropertiesDataSet MgmtID = 0x2003
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
	case MIDGrandmasterSettings:
		return "GRANDMASTER_SETTINGS_NP"
	}
	return fmt.Sprintf("0x%0x", uint16(mid))
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
	default:
		return nil, fmt.Errorf("unsupported management ID: 0x%04x", mid)
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
	TimeSource   uint8
}

func (GrandmasterSettings) MgmtID() MgmtID {
	return MIDGrandmasterSettings
}

func GrandmasterSettingsBinaryMsg(c *MsgPreparer, data GrandmasterSettings) ([]byte, error) {
	return MgmtSetBinaryMsg(c, data)
}
