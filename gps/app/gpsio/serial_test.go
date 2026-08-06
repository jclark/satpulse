package gpsio

import (
	"errors"
	"io"
	"testing"

	"github.com/jclark/satpulse/gps/lib/term"
)

type nonTTYFile struct{}

func (nonTTYFile) Read([]byte) (int, error)    { return 0, io.EOF }
func (nonTTYFile) Write(p []byte) (int, error) { return len(p), nil }
func (nonTTYFile) Close() error                { return nil }
func (nonTTYFile) Path() string                { return "not-a-tty" }
func (nonTTYFile) Buffered() (int, error)      { return 0, nil }

func TestModemControlLineStateRequiresTTY(t *testing.T) {
	c := newSerialConn(nonTTYFile{}, term.DevFIFO)
	if _, err := c.ModemControlLineState(); !errors.Is(err, term.ErrNotATTY) {
		t.Fatalf("ModemControlLineState error = %v, want ErrNotATTY", err)
	}
}
