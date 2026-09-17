package pmc

import (
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

type packetRead struct {
	data []byte
	err  error
}

type scriptedPacketConn struct {
	reads []packetRead
}

func (conn *scriptedPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if len(conn.reads) == 0 {
		return 0, nil, errors.New("unexpected read")
	}
	read := conn.reads[0]
	conn.reads = conn.reads[1:]
	return copy(p, read.data), nil, read.err
}

func (*scriptedPacketConn) WriteTo([]byte, net.Addr) (int, error) {
	return 0, errors.New("unexpected write")
}

func (*scriptedPacketConn) Close() error                     { return nil }
func (*scriptedPacketConn) LocalAddr() net.Addr              { return nil }
func (*scriptedPacketConn) SetDeadline(time.Time) error      { return nil }
func (*scriptedPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (*scriptedPacketConn) SetWriteDeadline(time.Time) error { return nil }

func responsePacket(t *testing.T, sequenceID, portNumber uint16) []byte {
	t.Helper()
	msg := NewMgmtSetMsg(NullPTPMgmt{})
	msg.SequenceID = sequenceID
	msg.TargetPortIdentity.PortNumber = portNumber
	msg.ActionField = ActionResponse
	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRecvSkipsMissingResponses(t *testing.T) {
	const (
		portNumber             = 1234
		firstMissingSequenceID = 1
		wantSequenceID         = 3
	)
	conn := &scriptedPacketConn{
		reads: []packetRead{
			{err: os.ErrDeadlineExceeded},
			{err: os.ErrDeadlineExceeded},
			{data: responsePacket(t, 1, portNumber)},
			{data: responsePacket(t, 2, portNumber)},
			{data: responsePacket(t, wantSequenceID, portNumber)},
			{data: responsePacket(t, wantSequenceID-1, portNumber)},
		},
	}
	client := &Client{
		MsgPreparer: MsgPreparer{PortNumber: portNumber},
		T:           &Transport{Conn: conn},
	}

	for sequenceID := uint16(firstMissingSequenceID); sequenceID < wantSequenceID; sequenceID++ {
		if _, err := client.Recv(sequenceID); !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("Recv(%d) error: got %v, want deadline exceeded", sequenceID, err)
		}
	}

	resp, err := client.Recv(wantSequenceID)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Prefix().SequenceID; got != wantSequenceID {
		t.Fatalf("sequence ID: got %d, want %d", got, wantSequenceID)
	}

	_, err = client.Recv(wantSequenceID + 1)
	if err == nil || !strings.Contains(err.Error(), "sequence ID mismatch") {
		t.Fatalf("unexpected error for response older than cleared marker: %v", err)
	}
}
