package uncmsg

import (
	"fmt"
	"strconv"
	"testing"
)

func TestVersion_BuildNumber(t *testing.T) {
	tests := []struct {
		swVersion string
		build     int // -1 means error expected
	}{
		{
			swVersion: "R4.10Build13504",
			build:     13504,
		},
		{
			swVersion: "13504",
			build:     13504,
		},
		{
			swVersion: "R5.20Build12345",
			build:     12345,
		},
		{
			swVersion: "999",
			build:     999,
		},
		{
			swVersion: "R4.10NoBuild123",
			build:     123,
		},
		{
			swVersion: "R4.10BuildXYZ",
			build:     -1,
		},
		{
			swVersion: "ABC",
			build:     -1,
		},
		{
			swVersion: "",
			build:     -1,
		},
		{
			swVersion: "R4.10NoSuchKeyword456",
			build:     -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.swVersion, func(t *testing.T) {
			// Convert string to [33]byte array
			var swVersionArray [33]byte
			copy(swVersionArray[:], tt.swVersion)
			
			v := &Version{
				SwVersion: swVersionArray,
			}
			
			got, err := v.BuildNumber()
			
			if tt.build == -1 {
				// Expect error
				if err == nil {
					t.Errorf("BuildNumber() expected error but got none")
				}
				return
			}
			
			// Expect success
			if err != nil {
				t.Errorf("BuildNumber() unexpected error = %v", err)
				return
			}
			
			if got != tt.build {
				t.Errorf("BuildNumber() = %v, want %v", got, tt.build)
			}
		})
	}
}

// fixupVersionValueForBin converts a Version with ASCII SwVersion format
// to binary SwVersion format for binary test comparison
func fixupVersionValueForBin(msg Msg) Msg {
	v := msg.(*Version)
	result := *v // Copy the struct
	
	// Extract build number from ASCII format
	buildNum, err := v.BuildNumber()
	if err != nil {
		panic(fmt.Sprintf("failed to extract build number: %v", err))
	}
	
	// Convert to binary format (just the build number as string)
	buildStr := strconv.Itoa(buildNum)
	
	// Clear SwVersion and copy build string
	result.SwVersion = [33]byte{}
	copy(result.SwVersion[:], buildStr)
	
	return &result
}