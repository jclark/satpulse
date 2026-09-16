package term

import (
	"testing"
	"time"
)

func TestSafeWriteTimePTY(t *testing.T) {
	path := newTestPTY(t)
	port, safe, err := Open(path, RawMode, NoFlowControl, Speed(115200), ReadTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer port.Close()
	if !safe.IsZero() {
		t.Errorf("PTY Open safe write time = %v, want zero", safe)
	}
	if safe, err = port.Change(Speed(38400)); err != nil || !safe.IsZero() {
		t.Fatalf("PTY Change = %v, %v, want zero, nil", safe, err)
	}
}
