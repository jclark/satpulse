package sdpcmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)


type Printer interface {
	Print(f *os.File)
}

var _ Printer = (*InterfaceInfo)(nil)

func Cmd(lg *slog.Logger, progName string, cmdName string, cmdArgs []string) (usage string, err error) {
	jsonl, help, usageFunc, err := parseFlags(cmdName, cmdArgs)
	if err != nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}
	if help {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}

	// Phase 1: List mode without interface
	sections, err := showAll()
	if err != nil {
		return
	}

	for i, sect := range sections {
		if jsonl {
			b, err := json.Marshal(sect)
			if err != nil {
				continue
			}
			fmt.Println(string(b))
		} else {
			if i > 0 {
				fmt.Println()
			}
			sect.Print(os.Stdout)
		}
	}

	return
}
