package qtmmsg

import "strings"

// ResponseKind classifies a PQTM response independently of any sent request.
type ResponseKind int

const (
	ResponseNotPQTM ResponseKind = iota // not a PQTM sentence
	ResponseOK                          // OK, no data (e.g. PQTMCFGPPS,OK)
	ResponseOKData                      // OK with data (e.g. PQTMCFGPPS,OK,1,1,...)
	ResponseError                       // ERROR with error code
	ResponseData                        // data without OK (e.g. PQTMVERNO,...)
)

// ResponseClass is the result of classifying a received PQTM sentence.
type ResponseClass struct {
	Kind     ResponseKind
	Sentence string // sentence name (e.g. "PQTMCFGPPS")
	Error    string // non-empty for ResponseError
}

// ClassifyResponse classifies a received NMEA payload (between $ and *).
// Returns ResponseNotPQTM if the payload is not a PQTM sentence.
//
// PQTM command responses use the same sentence name as the command.
// The second comma-separated field discriminates the response type:
//
//	Write command ack:   PQTMCFGPPS,OK                        -> ResponseOK
//	Query response:      PQTMCFGPPS,OK,1,1,100000,1000,0,0    -> ResponseOKData
//	Error:               PQTMCFGPPS,ERROR,1                    -> ResponseError
//	Data without OK:     PQTMVERNO,LG290P03...,2024/04/30,...  -> ResponseData
func ClassifyResponse(recv string) ResponseClass {
	name, rest, hasRest := strings.Cut(recv, ",")
	if !strings.HasPrefix(name, "PQTM") {
		return ResponseClass{Kind: ResponseNotPQTM}
	}
	if !hasRest {
		return ResponseClass{Kind: ResponseData, Sentence: name}
	}
	second, errCode, hasMore := strings.Cut(rest, ",")
	switch second {
	case "OK":
		if hasMore {
			return ResponseClass{Kind: ResponseOKData, Sentence: name}
		}
		return ResponseClass{Kind: ResponseOK, Sentence: name}
	case "ERROR":
		return ResponseClass{Kind: ResponseError, Sentence: name, Error: errorMessage(errCode)}
	}
	return ResponseClass{Kind: ResponseData, Sentence: name}
}

// RequestKind classifies a sent PQTM command.
type RequestKind int

const (
	RequestCommand RequestKind = iota // expects ResponseOK or ResponseError
	RequestQuery                      // expects ResponseOKData or ResponseError
	RequestVerno                      // expects ResponseData or ResponseError
)

// RequestClass is the result of classifying a sent PQTM command.
type RequestClass struct {
	Kind     RequestKind
	Sentence string // sentence name (e.g. "PQTMCFGPPS")
}

// ClassifyRequest classifies a sent NMEA payload (between $ and *).
//
// Classification logic:
//   - Contains ",W," -> RequestCommand (write)
//   - Contains ",R," -> RequestQuery (read)
//   - Sentence is PQTMVERNO -> RequestVerno
//   - Otherwise -> RequestCommand (default)
func ClassifyRequest(sent string) RequestClass {
	name, _, _ := strings.Cut(sent, ",")
	if strings.Contains(sent, ",W,") {
		return RequestClass{Kind: RequestCommand, Sentence: name}
	}
	if strings.Contains(sent, ",R,") {
		return RequestClass{Kind: RequestQuery, Sentence: name}
	}
	if name == "PQTMVERNO" {
		return RequestClass{Kind: RequestVerno, Sentence: name}
	}
	return RequestClass{Kind: RequestCommand, Sentence: name}
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
