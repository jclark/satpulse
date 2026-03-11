package uncmsg

import (
	"fmt"
	"strconv"
	"testing"
)

var versionTests = []dataTestCase{
	{
		name:        "VERSION message",
		binPacket:   mustHexDecode("aa44b5612500340100a04b09f00e73150000000000120300120000003133353034000000000000000000000000000000000000000000000000000000004852505430302d533130432d500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000323331303431353030303030312d4d443232413632323435313839383200000000000000000000000000000000000000000000000000000000000000000000000000666633626164393933393561396166630000000000000000000000000000000000323032342f30342f3033000000000000000000000000000000000000000000000000000000000000000000e108e8bf"),
		asciiPacket: "#VERSIONA,97,GPS,FINE,2379,359862000,0,0,18,3;\"UM980\",\"R4.10Build13504\",\"HRPT00-S10C-P\",\"2310415000001-MD22A6224518982\",\"ff3bad99395a9afc\",\"2024/04/03\"*8d58bb37\r\n",
		msg: &Msg{
			Hdr: MsgHdr{
				CPUIdlePercent: 97,
				TimingHdr: TimingHdr{
					TimeRef:            TimeRefGPS,
					TimeStatus:         TimeStatusFine,
					Week:               2379,
					MillisecondsOfWeek: 359862000,
					Version:            0,
					Reserved:           0,
					LeapSec:            18,
					DelayMs:            3,
				},
			},
			Body: &Version{
				Type:        ProductModelUM980,
				SwVersion:   [33]byte{'R', '4', '.', '1', '0', 'B', 'u', 'i', 'l', 'd', '1', '3', '5', '0', '4'},
				Auth:        [129]byte{'H', 'R', 'P', 'T', '0', '0', '-', 'S', '1', '0', 'C', '-', 'P'},
				Psn:         [66]byte{'2', '3', '1', '0', '4', '1', '5', '0', '0', '0', '0', '0', '1', '-', 'M', 'D', '2', '2', 'A', '6', '2', '2', '4', '5', '1', '8', '9', '8', '2'},
				EfuseID:     [33]byte{'f', 'f', '3', 'b', 'a', 'd', '9', '9', '3', '9', '5', 'a', '9', 'a', 'f', 'c'},
				CompileTime: [43]byte{'2', '0', '2', '4', '/', '0', '4', '/', '0', '3'},
			},
		},
		fixupMsgForBin: fixupVersionMsgForBin,
	},
}

func TestVersion(t *testing.T) {
	t.Run("bin", func(t *testing.T) {
		testDataBin(t, versionTests)
	})
	t.Run("ascii", func(t *testing.T) {
		testDataAscii(t, versionTests)
	})
}

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
func fixupVersionMsgForBin(msg *Msg) *Msg {
	v := msg.Body.(*Version)
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

	return &Msg{
		Hdr:  msg.Hdr,
		Body: &result,
	}
}
