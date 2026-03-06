package qtmmsg

import "strings"

// ResponseKind classifies a PQTM response.
type ResponseKind int

const (
	NotResponse   ResponseKind = iota
	ResponseOK                 // bare OK, no data
	ResponseData               // OK followed by data, or data without OK (e.g. PQTMVERNO)
	ResponseError              // ERROR with error code
)

// CheckResponse checks whether recv is a response to sent.
// Both are PQTM payloads (content between $ and *).
// Returns NotResponse if the sentence names do not match.
// For ResponseError, the string is the human-readable error message.
//
// PQTM command responses always use the same sentence name as the command.
// The second comma-separated field discriminates the response type:
//
//   Write command ack:   PQTMCFGPPS,OK                        -> ResponseOK
//   Query response:      PQTMCFGPPS,OK,1,1,100000,1000,0,0    -> ResponseData
//   Error:               PQTMCFGPPS,ERROR,1                    -> ResponseError
//   Data without OK:     PQTMVERNO,LG290P03...,2024/04/30,...  -> ResponseData
//
// The PQTMVERNO response is an exception: it returns data fields directly
// without an OK prefix. Any unrecognized second field is treated as data.
func CheckResponse(sent, recv string) (ResponseKind, string) {
	sentName, _, _ := strings.Cut(sent, ",")
	recvName, rest, hasRest := strings.Cut(recv, ",")
	if sentName != recvName {
		return NotResponse, ""
	}
	if !hasRest {
		return ResponseData, ""
	}
	second, errCode, hasMore := strings.Cut(rest, ",")
	switch second {
	case "OK":
		if hasMore {
			return ResponseData, ""
		}
		return ResponseOK, ""
	case "ERROR":
		return ResponseError, errorMessage(errCode)
	}
	return ResponseData, ""
}

func errorMessage(code string) string {
	switch code {
	case "1":
		return "invalid parameters"
	case "2":
		return "execution failed"
	case "3":
		return "unsupported command"
	}
	if code == "" {
		return "unknown error"
	}
	return code
}
