package sdbpbin

func init() {
	regMsg[DatGPST]("GPST")
	regMsg[DatTPPS]("TPPS")
}

// DatGPSTValid is a bitmask for DatGPST validity flags.
type DatGPSTValid uint8

const (
	DatGPSTTowValid  DatGPSTValid = 1 << 0
	DatGPSTWeekValid DatGPSTValid = 1 << 1
)

// DatGPST is DAT-GPST (class 0x06, id 0x17).
type DatGPST struct {
	LocalTimestamp uint32
	Valid          DatGPSTValid
	GPSWeek        uint16
	TOW            float64 // seconds of week
	TimeAccuracy   uint32  // nanoseconds; 0xFFFFFFFF means invalid
}

func (m *DatGPST) ID() MsgID { return makeMsgID(clsDAT, 0x17) }

// TimeValid reports whether the GPS time fields are valid.
func (m *DatGPST) TimeValid() bool {
	return m.Valid&(DatGPSTTowValid|DatGPSTWeekValid) == DatGPSTTowValid|DatGPSTWeekValid
}

// TPPSTimeRef identifies the time reference for DAT-TPPS.
type TPPSTimeRef uint8

const (
	TPPSTimeRefUTC TPPSTimeRef = 0
	TPPSTimeRefBDS TPPSTimeRef = 1
	TPPSTimeRefGPS TPPSTimeRef = 2
	TPPSTimeRefGLO TPPSTimeRef = 3
	TPPSTimeRefGAL TPPSTimeRef = 4
)

// TPPSUTCType identifies the UTC realization used when TimeRef is UTC.
type TPPSUTCType uint8

const (
	TPPSUTCInvalid TPPSUTCType = 0 // time reference != UTC or no UTC correction
	TPPSUTCBDS     TPPSUTCType = 1
	TPPSUTCGPS     TPPSUTCType = 2
	TPPSUTCGLO     TPPSUTCType = 3
	TPPSUUTCGAL    TPPSUTCType = 4
)

// DatTPPS is DAT-TPPS (class 0x06, id 0x41).
type DatTPPS struct {
	PPSIndex    uint8
	TOW         float64     // pulse time, seconds of week
	Week        uint16
	PPSResidual int32       // picoseconds
	TimeRef     TPPSTimeRef // time reference
	UTCType     TPPSUTCType // UTC source
}

func (m *DatTPPS) ID() MsgID { return makeMsgID(clsDAT, 0x41) }
