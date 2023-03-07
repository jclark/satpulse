package ubxmsg

import (
	"github.com/jclark/gps2phc/internal/ubx"
)

type Message struct {
	um ubx.Msg
}

func Parse(frame string) (*Message, error) {
	um, err := ubx.ParseMsg(frame)
	if err != nil {
		return nil, err
	}
	return &Message{um}, nil
}

func (m *Message) UBX() ubx.Msg {
	return m.um
}
