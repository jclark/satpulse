package as

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

func TestSpeedChange(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver(),
		prt: [2]uint32{115200, 115200}, liveRate: 115200}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetBaudRate(230400)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if rcvr.prt != [2]uint32{230400, 230400} {
		t.Errorf("PRT records = %v, want both 230400", rcvr.prt)
	}
	if rcvr.liveRate != 230400 {
		t.Errorf("live rate = %d, want 230400", rcvr.liveRate)
	}
	if b, ok := cfg.ConfigProps().GetBaudRate(); !ok || b != 230400 {
		t.Errorf("achieved baud = %d/%v, want 230400", b, ok)
	}
}

func TestSpeedChangeThenSave(t *testing.T) {
	// A combined --speed --save persists the NEW rate: the save runs
	// after the confirmed switch and its minimal mask covers baud.
	rcvr := &testReceiver{monVer: tau1201Ver(),
		prt: [2]uint32{115200, 115200}, liveRate: 115200}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetBaudRate(230400)
	target.Opts.Save = gpsprot.SaveMinimal
	_, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if len(rcvr.saves) != 1 || rcvr.saves[0].Mask&asbin.CfgCfgMaskBaudrate == 0 {
		t.Errorf("saves = %v, want one save covering baud", rcvr.saves)
	}
	if rcvr.liveRate != 230400 {
		t.Errorf("live rate = %d, want 230400 before the save", rcvr.liveRate)
	}
}

func TestShowPort(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver(),
		prt: [2]uint32{115200, 115200}, liveRate: 115200}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{Get: gpsprot.PropIDBaudRate}
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	props := cfg.ConfigProps()
	if b, ok := props.GetBaudRate(); !ok || b != 115200 {
		t.Errorf("baud = %d/%v, want 115200", b, ok)
	}
	if _, ok := props.GetPort(); ok {
		t.Error("port name reported; the active UART is not identifiable")
	}
}
