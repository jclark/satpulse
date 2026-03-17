package sdbpbin

var queVerID = makeMsgID(clsQUE, 0x01)

func init() {
	regMsg[QueVer]("VER")
}

// QueVer is QUE-VER (class 0x05, id 0x01).
// The payload is a variable-length ASCII string.
type QueVer struct {
	Version AsciiSlice `json:"version"`
}

func (m *QueVer) ID() MsgID { return queVerID }

func (m *QueVer) FixedPart() any { return nil }

func (m *QueVer) InitVaryingPart(payloadLen int) error {
	m.Version = make(AsciiSlice, payloadLen)
	return nil
}

func (m *QueVer) VaryingPart() any { return &m.Version }

var _ VaryingMsg = (*QueVer)(nil)
