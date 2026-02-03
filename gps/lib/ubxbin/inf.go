package ubxbin

const (
	InfDebugID   MsgID = clsInf | (0x04 << 8)
	InfErrorID   MsgID = clsInf | (0x00 << 8)
	InfNoticeID  MsgID = clsInf | (0x02 << 8)
	InfTestID    MsgID = clsInf | (0x03 << 8)
	InfWarningID MsgID = clsInf | (0x01 << 8)
)

type InfText []byte

func (m *InfText) InitVaryingPart(payloadLen int) error {
	*m = make([]byte, payloadLen)
	return nil
}

func (m *InfText) FixedPart() any {
	return nil
}

func (m *InfText) VaryingPart() any {
	return (*[]byte)(m)
}

type InfDebug struct{ InfText }

func (m *InfDebug) ID() MsgID { return InfDebugID }

type InfError struct{ InfText }

func (m *InfError) ID() MsgID { return InfErrorID }

type InfNotice struct{ InfText }

func (m *InfNotice) ID() MsgID { return InfNoticeID }

type InfTest struct{ InfText }

func (m *InfTest) ID() MsgID { return InfTestID }

type InfWarning struct{ InfText }

func (m *InfWarning) ID() MsgID { return InfWarningID }

func init() {
	regMsg[InfDebug]("DEBUG")
	regMsg[InfError]("ERROR")
	regMsg[InfNotice]("NOTICE")
	regMsg[InfTest]("TEST")
	regMsg[InfWarning]("WARNING")
}
