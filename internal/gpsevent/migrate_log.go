//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/gpsevent"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

// OldLogEvent represents the old log format
type OldLogEvent struct {
	T          time.Time              `json:"t"`
	Nanos      time.Duration          `json:"nanos"`
	Timestamp  *OldTimestamp          `json:"timestamp,omitempty"`
	Time       *gpsprot.TimeMsg       `json:"time,omitempty"`
	Survey     *gpsprot.SurveyMsg     `json:"survey,omitempty"`
	LeapSecond *gpsprot.LeapSecondMsg `json:"leapSecond,omitempty"`
	Satellites *gpsprot.SatellitesMsg `json:"satellites,omitempty"`
}

type OldTimestamp struct {
	T     ptime.Time    `json:"t"`
	Era   ptime.Era     `json:"era"`
	Delay time.Duration `json:"delay,omitempty"`
}

func migrateEvent(oldEvent OldLogEvent) gpsevent.LogEvent {
	newEvent := gpsevent.LogEvent{
		T:          oldEvent.T.UTC(), // Convert to UTC (old logs had timezone)
		Nanos:      oldEvent.Nanos,
		Time:       oldEvent.Time,
		Survey:     oldEvent.Survey,
		LeapSecond: oldEvent.LeapSecond,
		Satellites: oldEvent.Satellites,
	}
	// Migrate timestamp to PulseEdge
	if oldEvent.Timestamp != nil {
		// Old format only had timestamp and delay
		// We don't have the PHC read time, so we approximate:
		// TRead ≈ T + Delay
		tReadApprox := oldEvent.Timestamp.T.Add(oldEvent.Timestamp.Delay)
		newEvent.PulseEdge = &gpsevent.PulseEdge{
			T:     oldEvent.Timestamp.T,
			Era:   oldEvent.Timestamp.Era,
			TRead: tReadApprox, // approximation
		}
	}
	return newEvent
}

func main() {
	flag.Parse()

	if flag.NArg() != 2 {
		log.Fatalf("Usage: %s <input.jsonl> <output.jsonl>", os.Args[0])
	}

	inputFile := flag.Arg(0)
	outputFile := flag.Arg(1)

	// Open input
	in, err := os.Open(inputFile)
	if err != nil {
		log.Fatalf("Failed to open input: %v", err)
	}
	defer in.Close()

	// Create output
	out, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Failed to create output: %v", err)
	}
	defer out.Close()

	scanner := bufio.NewScanner(in)
	lineNum := 0
	migratedCount := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		// Try to unmarshal as old format
		var oldEvent OldLogEvent
		if err := json.Unmarshal(line, &oldEvent); err != nil {
			log.Printf("Warning: line %d: failed to parse: %v", lineNum, err)
			continue
		}

		// Migrate to new format
		newEvent := migrateEvent(oldEvent)

		// Marshal new format
		newLine, err := json.Marshal(newEvent)
		if err != nil {
			log.Printf("Warning: line %d: failed to marshal: %v", lineNum, err)
			continue
		}

		// Write to output
		fmt.Fprintf(out, "%s\n", newLine)
		migratedCount++
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %v", err)
	}

	log.Printf("Successfully migrated %d events from %d lines", migratedCount, lineNum)
}
