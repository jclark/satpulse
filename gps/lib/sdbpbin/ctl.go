package sdbpbin

func init() {
	regMsg[CtlRestart]("RESTART")
	regMsg[CtlConfig]("CONFIG")
	regMsg[CtlStandby]("STANDBY")
}

// CtlRestart is CTL-RESTART (class 0x02, id 0x01).
type CtlRestart struct {
	StartupMode uint8 // 1=cold, 2=warm, 3=hot
	ResetMode   uint8 // 0=soft reset, 1=hard reset
}

func (m *CtlRestart) ID() MsgID { return makeMsgID(clsCTL, 0x01) }

// Startup modes
const (
	StartupCold = 1
	StartupWarm = 2
	StartupHot  = 3
)

// Reset modes
const (
	ResetSoft = 0
	ResetHard = 1
)

// CtlConfig is CTL-CONFIG (class 0x02, id 0x02).
type CtlConfig struct {
	Action uint16
}

func (m *CtlConfig) ID() MsgID { return makeMsgID(clsCTL, 0x02) }

// Config actions
const (
	ConfigRestoreDefault = 0
	ConfigSaveBRAMFlash  = 1 // save to BRAM and Flash
	ConfigSaveBRAM       = 2 // save to BRAM only
)

// CtlStandby is CTL-STANDBY (class 0x02, id 0x04).
type CtlStandby struct {
	Mode uint8 // 1=enter standby
}

func (m *CtlStandby) ID() MsgID { return makeMsgID(clsCTL, 0x04) }
