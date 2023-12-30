package pmc

import (
	"fmt"
	"os"
)

const PTP4LSocketPath = "/var/run/ptp4l"

const localSocketPathFormat = "/var/run/satpulse-pmc%d.sock"

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

func (client *Client) Close() error {
	closeErr := client.T.Conn.Close()
	cleanErr := client.T.Cleanup()
	if closeErr != nil {
		return closeErr
	}
	return cleanErr
}

func (client *Client) SendRecv(msg MgmtMsg) (MgmtMsg, error) {
	seqid, err := client.Send(msg)
	if err != nil {
		return nil, err
	}
	return client.Recv(seqid)
}

func (client *Client) Send(msg MgmtMsg) (uint16, error) {
	client.PrepareMsg(msg)
	seqid := msg.Prefix().SequenceID
	data, err := msg.MarshalBinary()
	if err != nil {
		return seqid, fmt.Errorf("could not marshal PTP management message: %w", err)
	}
	_, err = client.T.Conn.WriteTo(data, client.T.RemoteAddr)
	if err != nil {
		return seqid, fmt.Errorf("could not send PTP management message: %w", err)
	}
	return seqid, nil
}

func (client *Client) Recv(seqid uint16) (MgmtMsg, error) {
	buf := make([]byte, 2048)
	n, _, err := client.T.Conn.ReadFrom(buf)
	if err != nil {
		return nil, fmt.Errorf("could not receive PTP management message: %w", err)
	}

	recvData := buf[:n]

	resp, err := UnmarshalMgmtMsg(recvData)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal message: %w", err)
	}
	respPrefix := resp.Prefix()
	if respPrefix.TargetPortIdentity.PortNumber != client.PortNumber {
		return nil, fmt.Errorf("unexpected port number in response, got %d", respPrefix.TargetPortIdentity.PortNumber)
	}
	if respPrefix.SequenceID != seqid {
		return nil, fmt.Errorf("sequence ID mismatch: expected %d, got %d", seqid, respPrefix.SequenceID)
	}
	if respPrefix.ActionField != ActionResponse {
		return nil, fmt.Errorf("unexpected action field in response, got %d", respPrefix.ActionField)
	}
	return resp, nil
}
