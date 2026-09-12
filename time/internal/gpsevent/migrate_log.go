//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/time/internal/gpsevent"
)

// OldLogEvent is the old sparse event-log format: one optional field per
// message type, with nanos as integer nanoseconds since session start.
type OldLogEvent struct {
	T          time.Time              `json:"t"`
	Nanos      time.Duration          `json:"nanos"`
	PulseEdge  *gpsevent.PHCPulseEdge `json:"pulseEdge,omitempty"`
	Time       *gpsprot.TimeMsg       `json:"time,omitempty"`
	PosGeo     *gpsprot.PosGeoMsg     `json:"posGeo,omitempty"`
	PosECEF    *gpsprot.PosECEFMsg    `json:"posECEF,omitempty"`
	VelGeo     *gpsprot.VelGeoMsg     `json:"velGeo,omitempty"`
	VelECEF    *gpsprot.VelECEFMsg    `json:"velECEF,omitempty"`
	Survey     *gpsprot.SurveyMsg     `json:"survey,omitempty"`
	LeapSecond *gpsprot.LeapSecondMsg `json:"leapSecond,omitempty"`
	Satellites *gpsprot.SatellitesMsg `json:"satellites,omitempty"`
	NavEpoch   *gpsprot.NavEpochMsg   `json:"navEpoch,omitempty"`
	CorReport  *gpsprot.CorReportMsg  `json:"corReport,omitempty"`
}

// migrateEvent converts an old sparse event into a new envelope event.
// It returns false when the old event carries no recognized payload.
func migrateEvent(old OldLogEvent) (gpsevent.LogEvent, bool) {
	e := gpsevent.LogEvent{T: old.T.UTC(), Mono: gpsprot.Duration(old.Nanos)}
	switch {
	case old.PulseEdge != nil:
		e.Type = "phcPulseEdge"
		e.Data = old.PulseEdge
	case old.Time != nil:
		e.Type, e.Data = old.Time.MsgType(), old.Time
	case old.PosGeo != nil:
		e.Type, e.Data = old.PosGeo.MsgType(), old.PosGeo
	case old.PosECEF != nil:
		e.Type, e.Data = old.PosECEF.MsgType(), old.PosECEF
	case old.VelGeo != nil:
		e.Type, e.Data = old.VelGeo.MsgType(), old.VelGeo
	case old.VelECEF != nil:
		e.Type, e.Data = old.VelECEF.MsgType(), old.VelECEF
	case old.Survey != nil:
		e.Type, e.Data = old.Survey.MsgType(), old.Survey
	case old.LeapSecond != nil:
		e.Type, e.Data = old.LeapSecond.MsgType(), old.LeapSecond
	case old.Satellites != nil:
		e.Type, e.Data = old.Satellites.MsgType(), old.Satellites
	case old.NavEpoch != nil:
		e.Type, e.Data = old.NavEpoch.MsgType(), old.NavEpoch
	case old.CorReport != nil:
		e.Type, e.Data = old.CorReport.MsgType(), old.CorReport
	default:
		return e, false
	}
	return e, true
}

func main() {
	flag.Parse()

	if flag.NArg() != 2 {
		log.Fatalf("Usage: %s <input.jsonl> <output.jsonl>", os.Args[0])
	}

	in, err := os.Open(flag.Arg(0))
	if err != nil {
		log.Fatalf("Failed to open input: %v", err)
	}
	defer in.Close()

	out, err := os.Create(flag.Arg(1))
	if err != nil {
		log.Fatalf("Failed to create output: %v", err)
	}
	defer out.Close()

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	lineNum := 0
	migratedCount := 0
	for scanner.Scan() {
		lineNum++
		var old OldLogEvent
		if err := json.Unmarshal(scanner.Bytes(), &old); err != nil {
			log.Printf("Warning: line %d: failed to parse: %v", lineNum, err)
			continue
		}
		newEvent, ok := migrateEvent(old)
		if !ok {
			log.Printf("Warning: line %d: no recognized payload, skipping", lineNum)
			continue
		}
		newLine, err := json.Marshal(newEvent)
		if err != nil {
			log.Printf("Warning: line %d: failed to marshal: %v", lineNum, err)
			continue
		}
		fmt.Fprintf(out, "%s\n", newLine)
		migratedCount++
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %v", err)
	}

	log.Printf("Successfully migrated %d events from %d lines", migratedCount, lineNum)
}
