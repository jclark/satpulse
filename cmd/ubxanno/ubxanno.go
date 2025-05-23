package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ubxanno: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err != nil {
			fmt.Println(string(line))
			continue
		}
		tag, hasTag := entry["tag"].(string)
		binHex, hasBin := entry["bin"].(string)
		if !hasTag || tag != "UBX" || !hasBin || binHex == "" {
			fmt.Println(string(line))
			continue
		}
		data, err := hex.DecodeString(binHex)
		if err != nil {
			fmt.Println(string(line))
			continue
		}
		msg, err := ubxbin.ParseMsg(string(data))
		if err != nil {
			fmt.Println(string(line))
			continue
		}
		entry["payload"] = msg
		if err := encoder.Encode(entry); err != nil {
			return fmt.Errorf("encode error: %w", err)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("scan error: %w", err)
	}
	return nil
}
