package ts

import (
	"os"
	"testing"
)

func TestValidateGenTS(t *testing.T) {
	want, err := os.ReadFile("validate.gen.ts")
	if err != nil {
		t.Fatalf("reading validate.gen.ts: %v\n\nRun: go generate ./gps/ts", err)
	}
	got, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("gpsprot JSON serialisation has changed; run: go generate ./gps/ts")
	}
}
