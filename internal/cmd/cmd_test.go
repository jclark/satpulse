package cmd

import (
	"flag"
	"testing"
	
)

func TestIntFlag(t *testing.T) {
	var speed IntFlag
	f := flag.NewFlagSet("test", flag.ContinueOnError)
	f.Var(&speed, "speed", "set the speed")
	args := []string{}
	err := f.Parse(args)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if speed.Value != nil {
		t.Error("missing speed wasn't nil")
	}

	f = flag.NewFlagSet("test", flag.ContinueOnError)
	f.Var(&speed, "speed", "set the speed")

	args = []string{"-speed", "123"}
	err = f.Parse(args)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if speed.Value == nil {
		t.Error("unexpected nil value for speed")
	}
	if *speed.Value != 123 {
		t.Errorf("Expected 123, got %d", *speed.Value)
	}

}
