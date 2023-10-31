package main

import (
	"flag"
	"testing"
)

func TestIntFlag(t *testing.T) {
	var speed intFlag
	f := flag.NewFlagSet("test", flag.ContinueOnError)
	f.Var(&speed, "speed", "set the speed")
	args := []string{}
	err := f.Parse(args)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if speed.value != nil {
		t.Error("missing speed wasn't nil")
	}

	f = flag.NewFlagSet("test", flag.ContinueOnError)
	f.Var(&speed, "speed", "set the speed")

	args = []string{"-speed", "123"}
	err = f.Parse(args)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if speed.value == nil {
		t.Error("unexpected nil value for speed")
	}
	if *speed.value != 123 {
		t.Errorf("Expected 123, got %d", *speed.value)
	}


}