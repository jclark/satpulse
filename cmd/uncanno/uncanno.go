package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jclark/satpulse/internal/gpsio"
	"github.com/jclark/satpulse/internal/unc"
	"github.com/jclark/satpulse/internal/uncmsg"
)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "uncanno: %v\n", err)
		os.Exit(1)
	}
}

func run(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	out := bufio.NewWriter(w)
	defer out.Flush()

	for scanner.Scan() {
		line := scanner.Bytes()
		processed, err := processLine(line)
		if err != nil {
			// Pass through unchanged on error
			processed = line
		}
		out.Write(processed)
		out.WriteByte('\n')
	}

	return scanner.Err()
}

func processLine(line []byte) ([]byte, error) {
	var entry gpsio.PacketLogEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil, err
	}

	// Only process UNCA and UNCB packets
	if entry.Tag != unc.TagAscii && entry.Tag != unc.TagBinary {
		return line, nil
	}

	var msg *uncmsg.Msg
	var err error

	switch entry.Tag {
	case unc.TagAscii:
		if entry.Ascii == "" {
			return line, nil
		}
		msg, err = uncmsg.ParseAsciiMessage([]byte(entry.Ascii))
	case unc.TagBinary:
		if len(entry.Bin) == 0 {
			return line, nil
		}
		msg, err = uncmsg.ParseBinMsg(entry.Bin)
	}

	if err != nil {
		return nil, err
	}

	// Don't add fields for unknown message types
	switch msg.Body.(type) {
	case *uncmsg.UnknownBinMsgBody, *uncmsg.UnknownAsciiMsgBody:
		return line, nil
	}

	// Insert header and payload fields into the JSON
	return insertFields(line, msg)
}

func insertFields(line []byte, msg *uncmsg.Msg) ([]byte, error) {
	// Marshal header and payload
	headerJSON, err := json.Marshal(&msg.Hdr)
	if err != nil {
		return nil, err
	}
	payloadJSON, err := json.Marshal(msg.Body)
	if err != nil {
		return nil, err
	}

	// Find the closing brace
	closeIdx := bytes.LastIndexByte(line, '}')
	if closeIdx == -1 {
		return nil, fmt.Errorf("invalid JSON: no closing brace")
	}

	// Build the new fields string
	newFields := fmt.Sprintf(",\"header\":%s,\"payload\":%s", headerJSON, payloadJSON)

	// Insert the new fields before the closing brace
	result := make([]byte, 0, len(line)+len(newFields))
	result = append(result, line[:closeIdx]...)
	result = append(result, newFields...)
	result = append(result, line[closeIdx:]...)

	return result, nil
}