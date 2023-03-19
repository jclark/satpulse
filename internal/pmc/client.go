package pmc

import (
	"fmt"
	"os"
)

const PTP4LSocketPath = "/var/run/ptp4l"

const localSocketPathFormat = "/tmp/gps2phc%d.sock"

type Client struct {
	MsgPreparer
	T *Transport
}

type ClientConfig struct {
	RemoteSocketPath      string
	LocalSocketPathFormat string
}

func NewClientConfig() *ClientConfig {
	c := &ClientConfig{}
	c.SetDefaults()
	return c
}

func (cfg *ClientConfig) SetDefaults() {
	cfg.RemoteSocketPath = PTP4LSocketPath
	cfg.LocalSocketPathFormat = localSocketPathFormat
}

func NewClient(cfg *ClientConfig) (*Client, error) {
	if cfg == nil {
		cfg = &ClientConfig{}
		cfg.SetDefaults()
	}
	pid := os.Getpid()
	t, err := NewUnixTransport(fmt.Sprintf(cfg.LocalSocketPathFormat, pid), cfg.RemoteSocketPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		MsgPreparer: MsgPreparer{PortNumber: uint16(pid)},
		T:           t,
	}, nil
}

func (client *Client) Send(msg MgmtMsg) (MgmtMsg, error) {
	client.PrepareMsg(msg)
	data, err := msg.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("could not marshal message: %w", err)
	}
	_, err = client.T.Write(data)
	if err != nil {
		return nil, fmt.Errorf("could not write message: %w", err)
	}

	buf := make([]byte, 2048)
	n, _, err := client.T.Conn.ReadFrom(buf)
	if err != nil {
		return nil, fmt.Errorf("could not read message: %w", err)
	}

	recvData := buf[:n]

	m, err := UnmarshalMgmtMsg(recvData)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal message: %w", err)
	}
	return m, nil
}
