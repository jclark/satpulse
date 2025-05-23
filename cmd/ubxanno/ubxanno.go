package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
	"github.com/jclark/satpulse/internal/ubxcfgval"
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
		out, _ := entry["out"].(bool)
		entry["payload"] = msg
		cfgData, cdt := msgCfgData(msg, out)
		if cfgData != nil {
			var encodedCfgData interface{}
			if cdt == cfgDataItems {
				encodedCfgData, err = encodeCfgDataItems(cfgData)
			} else {
				encodedCfgData, err = encodeCfgDataKeys(cfgData)
			}
			if err == nil {
				entry["cfgData"] = encodedCfgData
			}
		}
		if err := encoder.Encode(entry); err != nil {
			return fmt.Errorf("encode error: %w", err)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("scan error: %w", err)
	}
	return nil
}

func encodeCfgDataItems(cfgData []byte) (interface{}, error) {
	items, err := ubxcfgval.UnmarshalItems(cfgData)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func encodeCfgDataKeys(cfgData []byte) (interface{}, error) {
	keys, err := ubxcfgval.UnmarshalKeys(cfgData)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

type cfgDataType int

const (
	cfgDataItems cfgDataType = iota
	cfgDataKeys
)

func msgCfgData(msg ubxbin.Msg, out bool) ([]byte, cfgDataType) {
	cfd := cfgDataItems
	switch mt := msg.(type) {
	case *ubxbin.CfgValget:
		if out {
			cfd = cfgDataKeys
		}
		return mt.CfgData, cfd
	case *ubxbin.CfgValset:
		return mt.CfgData, cfd
	case *ubxbin.CfgValdel:
		cfd = cfgDataKeys
		return mt.CfgData, cfd
	default:
		return nil, 0
	}
}
