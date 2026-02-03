//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
)

func main() {
	for _, s := range os.Args[1:] {
		fmt.Print(wrap(s))
	}
}

func wrap(data string) string {
	return fmt.Sprintf("$%s*%02X\r\n", data, nmeamsg.Checksum(data))
}
