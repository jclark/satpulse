package sdbpbin

// DatTimeValid is a bitmask for DAT-*T validity flags.
type DatTimeValid uint8

const (
	DatTimeTowValid DatTimeValid = 1 << iota
	DatTimeWeekValid
)

// DatGNSST is the common layout for DAT-BDST, DAT-GPST, and DAT-GALT (19 bytes).
type DatGNSST struct {
	LocalTimestamp uint32
	Valid          DatTimeValid
	Week           uint16
	TOW            float64 // seconds of week
	TimeAccuracy   uint32  // nanoseconds; 0xFFFFFFFF means invalid
}

// TimeValid reports whether the time fields are valid.
func (m *DatGNSST) TimeValid() bool {
	return m.Valid&(DatTimeTowValid|DatTimeWeekValid) == DatTimeTowValid|DatTimeWeekValid
}

// DatBDST is DAT-BDST (class 0x06, id 0x16). BeiDou time.
type DatBDST struct{ DatGNSST }

func (m *DatBDST) ID() MsgID { return makeMsgID(clsDAT, 0x16) }

// DatGPST is DAT-GPST (class 0x06, id 0x17). GPS time.
type DatGPST struct{ DatGNSST }

func (m *DatGPST) ID() MsgID { return makeMsgID(clsDAT, 0x17) }

// DatGALT is DAT-GALT (class 0x06, id 0x19). Galileo time.
type DatGALT struct{ DatGNSST }

func (m *DatGALT) ID() MsgID { return makeMsgID(clsDAT, 0x19) }

// UTCType identifies the UTC realization used when TimeRef is UTC.
type UTCType uint8

const (
	UTCInvalid UTCType = 0 // time reference != UTC or no UTC correction
	UTCBDS     UTCType = 1
	UTCGPS     UTCType = 2
	UTCGLO     UTCType = 3
	UTCGAL     UTCType = 4
)

// DatTPPS is DAT-TPPS (class 0x06, id 0x41).
type DatTPPS struct {
	PPSIndex    uint8
	TOW         float64 // pulse time, seconds of week
	Week        uint16
	PPSResidual int32   // picoseconds
	TimeRef     TimeRef // time reference
	UTCType     UTCType // UTC source
}

func (m *DatTPPS) ID() MsgID { return makeMsgID(clsDAT, 0x41) }

func init() {
	regMsg[DatBDST]("BDST")
	regMsg[DatGPST]("GPST")
	regMsg[DatGALT]("GALT")
	regMsg[DatTPPS]("TPPS")
}
