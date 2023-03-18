package pmc

import "os"

type MgmtClient struct {
	sequenceID uint16
	portNumber uint16
	domain     uint8
}

func NewMgmtClient() *MgmtClient {
	return &MgmtClient{
		portNumber: uint16(os.Getpid()),
	}
}

func (c *MgmtClient) PrepareMsg(m MgmtMsg) {
	h := &m.Prefix().Header
	h.DomainNumber = c.domain
	h.SourcePortIdentity.PortNumber = c.portNumber
	h.SequenceID = c.getSequenceID()
}

func (c *MgmtClient) getSequenceID() uint16 {
	id := c.sequenceID
	c.sequenceID++
	return id
}

func MgmtSetBinaryMsg[D MgmtData](c *MgmtClient, data D) ([]byte, error) {
	msg := NewMgmtSetMsg(data)
	c.PrepareMsg(msg)
	return msg.MarshalBinary()
}
