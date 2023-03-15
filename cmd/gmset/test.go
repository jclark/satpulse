package main

import (
	"fmt"
	"os"

	"golang.org/x/exp/slices"
)

var header string = "\x0d\x02\x00\x3e\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x31\x4d\x00\x00\x04\x7f"
var body string = "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00\x01\x00"
var tlv string = "\x00\x01\x00\x0a\xc0\x01"
var grandmaster_settings_np string = "\x06\x23\xff\xff\x00\x25\x1c\xa0"

var msg = header + body + tlv + grandmaster_settings_np

const testPID = 12621

func run() error {
	client := NewManagementClient()
	client.portNumber = testPID
	gsn := NewGrandmasterSettingsNPMsg(client)
	bytes, err := gsn.MarshalBinary()
	if err != nil {
		return fmt.Errorf("first MarshalBinaryFailed: %v", err)
	}
	if len(msg) != len(bytes) {
		return fmt.Errorf("wrong length: got %d, want %d", len(bytes), len(msg))
	}
	for i := 0; i < len(msg); i++ {
		if msg[i] != bytes[i] {
			fmt.Printf("wrong byte at %d: got %02x, want %02x\n", i, bytes[i], msg[i])
		}
	}
	gsn.SetLength(uint16(len(bytes)))
	gsn2 := new(GrandmasterSettingsNPMsg)
	err = gsn2.UnmarshalBinary(bytes)
	if err != nil {
		return fmt.Errorf("UnmarshalBinary failed: %v", err)
	}
	if gsn.TLV.ValueField.Data != gsn2.TLV.ValueField.Data {
		return fmt.Errorf("data not round tripped: got %v, want %v", gsn2.TLV.ValueField.Data, gsn.TLV.ValueField.Data)
	}
	bytes2, err := gsn2.MarshalBinary()
	if err != nil {
		return err
	}
	if !slices.Equal(bytes, bytes2) {
		return fmt.Errorf("got different bytes: got %v, want %v", bytes2, bytes)
	}
	return nil
}

func main() {
	err := run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
