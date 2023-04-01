package ubx

import (
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

type Message struct {
	um bin.Msg
}

func Parse(frame string) (*Message, error) {
	um, err := bin.ParseMsg(frame)
	if err != nil {
		return nil, err
	}
	return &Message{um}, nil
}

func (m *Message) UBX() bin.Msg {
	return m.um
}
