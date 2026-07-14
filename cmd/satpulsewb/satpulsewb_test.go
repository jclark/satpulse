package main

import "testing"

func TestParseFlagsNoOpen(t *testing.T) {
	v, _, err := parseFlags([]string{"--no-open"})
	if err != nil {
		t.Fatal(err)
	}
	if !v.noOpen {
		t.Error("noOpen false")
	}
}
