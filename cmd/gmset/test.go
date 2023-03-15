package main

import (
	"fmt"
	"os"

	"golang.org/x/exp/slices"
)

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
