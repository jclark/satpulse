package asbin

const (
	RxmDumpRawID MsgID = clsRxm | (0x01 << 8)
	RxmRawID     MsgID = clsRxm | (0x57 << 8)
)

// RXM-DUMPRAW (0x02 0x01) - enable/disable raw data output. The raw
// data itself arrives as RXM-RAW (0x02 0x57) frames, roughly one per
// second, in a format the protocol documentation does not describe.
// Enabling is acknowledged like a CFG set; the TAU951M NAKs the
// disable yet applies it.
type RxmDumpRaw struct {
	Enable uint8 // 0=disable, 1=enable
}

func (m *RxmDumpRaw) ID() MsgID { return RxmDumpRawID }

func init() {
	regMsg[RxmDumpRaw]("DUMPRAW")
	idNameMap[RxmRawID] = "RAW"
}
