package airmsg

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ResponseKind classifies a PAIR response independently of any sent request.
type ResponseKind int

const (
	ResponseNotPAIR ResponseKind = iota // not a PAIR sentence
	ResponseOK                          // PAIR001 with result 0
	ResponseWait                        // PAIR001 with result 1
	ResponseError                       // PAIR001 with result >= 2
	ResponseData                        // data echoing command ID (e.g. PAIR073,5)
)

// ResponseClass is the result of classifying a received PAIR sentence.
type ResponseClass struct {
	Kind      ResponseKind
	CommandID string // 3-digit command ID (e.g. "073")
	Error     string // non-empty for ResponseError
}

// ClassifyResponse classifies a received NMEA payload (between $ and *).
// Returns ResponseNotPAIR if the payload is not a PAIR sentence.
//
// PAIR uses a universal ack message (PAIR001) that carries the original
// command's 3-digit ID and a numeric result code. Data responses use
// the original command's sentence name (e.g. PAIR073,5).
//
//	Ack success:     PAIR001,073,0  -> ResponseOK
//	Ack processing:  PAIR001,073,1  -> ResponseWait
//	Ack error:       PAIR001,073,3  -> ResponseError
//	Data response:   PAIR073,5      -> ResponseData
func ClassifyResponse(recv string) ResponseClass {
	name, rest, hasRest := strings.Cut(recv, ",")
	if len(name) < 4 || name[:4] != "PAIR" {
		return ResponseClass{Kind: ResponseNotPAIR}
	}
	if name == "PAIR001" {
		if !hasRest {
			return ResponseClass{Kind: ResponseNotPAIR}
		}
		cmdID, resultStr, hasResult := strings.Cut(rest, ",")
		if !hasResult {
			return ResponseClass{Kind: ResponseNotPAIR}
		}
		switch resultStr {
		case "0":
			return ResponseClass{Kind: ResponseOK, CommandID: cmdID}
		case "1":
			return ResponseClass{Kind: ResponseWait, CommandID: cmdID}
		default:
			n, err := strconv.Atoi(resultStr)
			if err != nil || n < 0 {
				return ResponseClass{Kind: ResponseNotPAIR}
			}
			return ResponseClass{Kind: ResponseError, CommandID: cmdID, Error: errorMessage(n)}
		}
	}
	// Data response: PAIR followed by 3-digit ID.
	if len(name) == 7 {
		return ResponseClass{Kind: ResponseData, CommandID: name[4:7]}
	}
	return ResponseClass{Kind: ResponseNotPAIR}
}

// RequestKind classifies a sent PAIR command.
type RequestKind int

const (
	RequestCommand RequestKind = iota // expects PAIR001 only
	RequestQuery                      // expects PAIR001 + data
)

// RequestClass is the result of classifying a sent PAIR command.
type RequestClass struct {
	Kind      RequestKind
	CommandID string // 3-digit command ID (e.g. "073")
}

// pairParityExceptions lists command IDs where even/odd parity gives
// the wrong answer. Sorted for binary search.
var pairParityExceptions = []int{
	3, 5, 7, 8, 11, 20, 23, 30, 412, 507, 508, 511, 513,
	596, 753, 755, 756, 757, 758, 906, 907, 908,
}

// ClassifyRequest classifies a sent NMEA payload (between $ and *).
// The default is even = command, odd = query. An internal exception
// list overrides this for cases where parity does not match.
func ClassifyRequest(sent string) RequestClass {
	name, _, _ := strings.Cut(sent, ",")
	if len(name) < 7 || name[:4] != "PAIR" {
		return RequestClass{CommandID: ""}
	}
	idStr := name[4:7]
	n, err := strconv.Atoi(idStr)
	if err != nil {
		return RequestClass{CommandID: idStr}
	}
	isQuery := n%2 != 0 // odd = query by default
	i := sort.SearchInts(pairParityExceptions, n)
	if i < len(pairParityExceptions) && pairParityExceptions[i] == n {
		isQuery = !isQuery // exception: flip
	}
	kind := RequestCommand
	if isQuery {
		kind = RequestQuery
	}
	return RequestClass{Kind: kind, CommandID: idStr}
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
