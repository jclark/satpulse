//go:build ignore

package main

import (
	"fmt"
	"log"

	"github.com/jclark/satpulse/gps/lib/ntptime"
)

func main() {
	state, err := ntptime.Get()
	if err != nil {
		log.Fatalf("Failed to get NTP state: %v", err)
	}
	leapStatus := "normal"
	switch state.LeapSecondStatus {
	case ntptime.LeapSecondInsTonight:
		leapStatus = "positive leap second at end of UTC day"
	case ntptime.LeapSecondDelTonight:
		leapStatus = "negative leap second at end of UTC day"
	case ntptime.LeapSecondInProgress:
		leapStatus = "currently in a leap second"
	case ntptime.LeapSecondRecent:
	}
	fmt.Printf("System Clock NTP State:\n")
	fmt.Printf("  Time:                %s\n", state.Time.Format("2006-01-02 15:04:05.00 -07:00"))
	fmt.Printf("  Synchronized:        %t\n", state.Synchronized)
	fmt.Printf("  Estimated Error:     %v\n", state.EstError)
	fmt.Printf("  Maximum Error:       %v\n", state.MaxError)
	fmt.Printf("  TAI Offset:          %d seconds\n", state.TAIOffset)
	fmt.Printf("  Leap Second Status:  %s\n", leapStatus)
}
