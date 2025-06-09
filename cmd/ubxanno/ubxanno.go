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
	schema := ubxcfgval.NewSchemaWithMsgout(ubxcfgval.GetDfltSchema())
	if err := run(schema); err != nil {
		fmt.Fprintf(os.Stderr, "ubxanno: %v\n", err)
		os.Exit(1)
	}
}

func run(schema *ubxcfgval.Schema) error {
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
		if _, isUnknown := msg.(*ubxbin.UnknownMsg); !isUnknown {
			entry["payload"] = msg
		}
		cfgData, cdt := msgCfgData(msg, out)
		if cfgData != nil {
			var encodedCfgData interface{}
			if cdt == cfgDataItems {
				encodedCfgData, err = encodeCfgDataItems(schema, cfgData)
			} else {
				encodedCfgData, err = encodeCfgDataKeys(schema, cfgData)
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

func encodeCfgDataItems(schema *ubxcfgval.Schema, cfgData []byte) (interface{}, error) {
	keys, values, err := schema.UnmarshalItemsFlat(cfgData)
	if err != nil {
		return nil, err
	}
	items := make([][2]interface{}, len(keys))
	for i := range keys {
		items[i] = [2]interface{}{keys[i], values[i]}
	}
	return items, nil
}

func encodeCfgDataKeys(schema *ubxcfgval.Schema, cfgData []byte) (interface{}, error) {
	return schema.UnmarshalKeysFlat(cfgData)
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
