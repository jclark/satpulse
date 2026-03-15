package airmsg

import (
	"fmt"
	"strconv"
	"strings"
)

// ResponseKind classifies a PAIR response.
type ResponseKind int

const (
	NotResponse     ResponseKind = iota
	ResponseOK                   // PAIR001 ack with result 0 (success)
	ResponseWait                 // PAIR001 ack with result 1 (processing, wait for final result)
	ResponseData                 // data reply echoing the command ID
	ResponseError                // PAIR001 ack with error result
	ResponseInvalid              // PAIR001 with matching command ID but unparseable result
)

// CheckResponse checks whether recv is a response to sent.
// Both are PAIR payloads (content between $ and *).
// Returns NotResponse if the message is unrelated to sent.
//
// PAIR uses a universal ack message (PAIR001) that carries the original
// command's 3-digit ID and a numeric result code. Data responses use
// the original command's sentence name (e.g. PAIR073 for a query sent
// as PAIR073).
//
//	Ack success:     PAIR001,073,0  -> ResponseOK
//	Ack processing:  PAIR001,073,1  -> ResponseWait
//	Ack error:       PAIR001,073,3  -> ResponseError
//	Data response:   PAIR073,5      -> ResponseData
func CheckResponse(sent, recv string) (ResponseKind, string) {
	if len(sent) < 7 || sent[:4] != "PAIR" {
		return NotResponse, ""
	}
	sentID := sent[4:7]
	recvName, rest, hasRest := strings.Cut(recv, ",")
	if recvName == "PAIR001" {
		// Universal ack: PAIR001,<cmdID>,<result>
		if !hasRest {
			return NotResponse, ""
		}
		cmdID, resultStr, hasResult := strings.Cut(rest, ",")
		if cmdID != sentID {
			return NotResponse, ""
		}
		if !hasResult {
			return ResponseInvalid, ""
		}
		switch resultStr {
		case "0":
			return ResponseOK, ""
		case "1":
			return ResponseWait, ""
		default:
			n, err := strconv.Atoi(resultStr)
			if err != nil || n < 0 {
				return ResponseInvalid, ""
			}
			return ResponseError, errorMessage(n)
		}
	}
	// Data response: same command ID echoed back (e.g. PAIR073,5)
	if len(recvName) == 7 && recvName[:4] == "PAIR" && recvName[4:7] == sentID {
		return ResponseData, ""
	}
	return NotResponse, ""
}

func errorMessage(code int) string {
	switch code {
	case 2:
		return "sending failed"
	case 3:
		return "command not supported"
	case 4:
		return "parameter error"
	case 5:
		return "service busy"
	}
	return fmt.Sprintf("unknown error %d", code)
}
