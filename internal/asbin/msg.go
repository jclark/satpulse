package asbin

// These are used in CfgMsg
const (
	NmeaGgaID MsgID = clsNmea | (iota << 8)
	NmeaGllID
	NmeaGsaID
	NmeaGrsID
	NmeaGsvID
	NmeaRmcID
	NmeaVtgID
	NmeaZdaID
	NmeaTxtID MsgID = clsNmea | (0x20 << 8)
)
