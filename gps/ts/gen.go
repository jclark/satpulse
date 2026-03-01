//go:build ignore

// gen.go writes validate.gen.ts and runs tsc to validate it.
// Usage: go generate ./gps/ts
package main

import (
	"fmt"
	"os"
	"os/exec"

	ts "github.com/jclark/satpulse/gps/ts"
)

func main() {
	data, err := ts.Generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile("validate.gen.ts", data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote validate.gen.ts")
	cmd := exec.Command("npx", "tsc", "--noEmit")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tsc failed:", err)
		os.Exit(1)
	}
	fmt.Println("tsc --noEmit passed")
}
