//go:build !windows

package main

import "github.com/spf13/pflag"

type platformVars struct{}

const platformSummary = ""

func registerPlatformFlags(*pflag.FlagSet, *flagVars) {}

// platformMain handles platform-specific run modes; there are none except
// on Windows, so the caller falls through to the foreground path.
func platformMain(string, *flagVars) {}
