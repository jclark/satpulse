package msgfile

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
)

// requestAnalyzer produces a requestAnalysis from outgoing message bytes.
// Implemented by each message type (UBXMsg, CASBINMsg, etc.).
type requestAnalyzer interface{}

// AckExpectation describes what ACK behavior a request expects.
type AckExpectation int

const (
	ExpectAckNone    AckExpectation = iota // no ack or nak expected
	ExpectAckOrNak                         // expect ack on success, nak on failure
	ExpectAckNakOnly                       // no ack on success, may get nak on failure
)

type dataExpectation int

const (
	expectDataUnknown  dataExpectation = iota // zero value: don't know what to expect
	expectDataNone                            // no data expected
	expectDataWithAck                         // data combined in ack packet
	expectDataAmbig                           // single data, indistinguishable from periodic
	expectDataSingle                          // one separate data response, confirmable
	expectDataMultiple                        // multiple data responses
)

type requestAnalysis struct {
	ackTag       gpsprot.Tag // tag where ACK/NAK arrives; empty if ExpectAckNone
	ackCorrelate string      // fragment for matching against response ackCorrelate
	expectAck    AckExpectation
	expectData   dataExpectation
	dataTag      gpsprot.Tag            // tag where data arrives; empty = any tag
	dataMatch    func(data string) bool // nil: accept all on matching tag; non-nil: true = matches
}

type responseKind int

const (
	responseMaybeData responseKind = iota // zero value: might be data, uncertain
	responseNotData                       // definitely not data
	responseInfo                          // informational, display without correlation
	responseData                          // confirmed data response
	responseAck                           // positive acknowledgement
	responseNak                           // negative acknowledgement
	responseWait                          // interim "still processing"
)

type responseAnalysis struct {
	kind         responseKind
	ackCorrelate string // ack/nak/wait: correlation fragment from packet
	ackError     string // nak: error description
}

type responseAnalyzer interface {
	analyzeResponse(data string) responseAnalysis
}

type ackStatus int

const (
	ackNotExpected ackStatus = iota // zero value: no ack expected
	ackWait                         // expecting ack
	ackWaitMore                     // got responseWait, real ack coming
	ackSuccess                      // got responseAck
	ackFailed                       // got responseNak
)

type dataStatus int

const (
	dataNotExpected dataStatus = iota // zero value: no data expected
	dataWait                          // expecting data
	dataReceived                      // got data
)

type requestState struct {
	msg      *RawMsg
	analysis requestAnalysis
	ack      ackStatus
	ackError string
	data     dataStatus
}

// RelevanceLevel classifies how relevant a received packet is.
type RelevanceLevel int

const (
	LevelAckOnly       RelevanceLevel = iota // ack/nak/wait packet, no content to display
	LevelNotResponse                         // not a response to anything sent
	LevelMaybeResponse                       // might be a response, uncertain
	LevelAmbigResponse                       // definitely a response, but ambiguous which request
	LevelMultiResponse                       // one of multiple data responses
	LevelSoleResponse                        // the single expected data response
)

// AckKind classifies the ACK type of a correlated packet.
type AckKind int

const (
	AckNone  AckKind = iota // not an ack/nak/wait
	AckAck                  // positive acknowledgement
	AckNak                  // negative acknowledgement
	AckOther                // other ack-related (e.g. interim "still processing")
)

// Correlation is the result of correlating a received packet.
type Correlation struct {
	Ack          AckKind
	NakError     string  // meaningful when Ack == AckNak; non-empty = error detail
	InResponseTo *RawMsg // non-nil when Ack != AckNone
	Relevance    RelevanceLevel
}

// Correlator combines request and response analyses to correlate
// received packets with sent messages.
type Correlator struct {
	analyzers map[gpsprot.Tag]responseAnalyzer
	requests  []requestState
}

// NewCorrelator creates a Correlator with the default response analyzers.
func NewCorrelator() *Correlator {
	return &Correlator{
		analyzers: map[gpsprot.Tag]responseAnalyzer{
			gpsreg.TagUBX:          ubxAnalyzer{},
			gpsreg.TagCASICBin:     casbinAnalyzer{},
			gpsreg.TagAllystarBin:  asbinAnalyzer{},
			gpsreg.TagSDBP:         sdbpAnalyzer{},
			gpsreg.TagNMEA:         nmeaAnalyzer{},
			gpsreg.TagUnicoreAscii: uncaAnalyzer{},
		},
	}
}

// NotifyMsgSent records that a message has been sent.
func (c *Correlator) NotifyMsgSent(rm RawMsg) {
	a := c.analyzeRequest(rm)
	var ack ackStatus
	switch a.expectAck {
	case ExpectAckOrNak, ExpectAckNakOnly:
		ack = ackWait
	}
	var data dataStatus
	switch a.expectData {
	case expectDataNone:
		data = dataNotExpected
	default:
		if a.expectData != expectDataUnknown || a.expectAck == ExpectAckNone {
			data = dataWait
		}
	}
	// expectDataUnknown with ack: data status depends on ack type,
	// but for now we set dataWait -- all expectDataUnknown messages
	// have ExpectAckNone.
	if a.expectData == expectDataUnknown {
		data = dataWait
	}
	c.requests = append(c.requests, requestState{
		msg:      &rm,
		analysis: a,
		ack:      ack,
		data:     data,
	})
}

func (c *Correlator) analyzeRequest(rm RawMsg) requestAnalysis {
	if ra, ok := rm.source.(interface {
		analyzeRequest(data string) requestAnalysis
	}); ok {
		return ra.analyzeRequest(string(rm.Bytes))
	}
	// No analyzer: unknown expectations.
	return requestAnalysis{}
}

// ReadyToSend checks whether sending rm would create an ambiguous
// ACK correlation with any pending request.
func (c *Correlator) ReadyToSend(rm RawMsg) bool {
	a := c.analyzeRequest(rm)
	if a.expectAck == ExpectAckNone || a.ackCorrelate == "" {
		return true
	}
	for i := range c.requests {
		rs := &c.requests[i]
		if c.requestComplete(rs) {
			continue
		}
		if rs.ack != ackWait && rs.ack != ackWaitMore {
			continue
		}
		if rs.analysis.ackTag == a.ackTag && rs.analysis.ackCorrelate == a.ackCorrelate {
			return false
		}
	}
	return true
}

// CorrelatePacket classifies and correlates a received packet.
func (c *Correlator) CorrelatePacket(tag gpsprot.Tag, data string) Correlation {
	ra := c.classifyResponse(tag, data)
	switch ra.kind {
	case responseAck, responseNak, responseWait:
		return c.correlateAck(tag, ra)
	case responseData:
		return c.correlateData(tag, data, true)
	case responseMaybeData:
		return c.correlateData(tag, data, false)
	case responseInfo:
		return Correlation{Relevance: LevelMaybeResponse}
	case responseNotData:
		return Correlation{Relevance: LevelNotResponse}
	}
	return Correlation{Relevance: LevelNotResponse}
}

func (c *Correlator) classifyResponse(tag gpsprot.Tag, data string) responseAnalysis {
	if a, ok := c.analyzers[tag]; ok {
		return a.analyzeResponse(data)
	}
	return responseAnalysis{kind: responseMaybeData}
}

func (c *Correlator) correlateAck(tag gpsprot.Tag, ra responseAnalysis) Correlation {
	var matches []*requestState
	for i := range c.requests {
		rs := &c.requests[i]
		if c.requestComplete(rs) {
			continue
		}
		if rs.ack != ackWait && rs.ack != ackWaitMore {
			continue
		}
		if rs.analysis.ackTag != tag {
			continue
		}
		if rs.analysis.ackCorrelate == ra.ackCorrelate {
			matches = append(matches, rs)
		}
	}
	if len(matches) != 1 {
		// Ambiguous or no match.
		cor := Correlation{Relevance: LevelNotResponse}
		if len(matches) > 1 {
			cor.Relevance = LevelAmbigResponse
		}
		return cor
	}
	rs := matches[0]
	switch ra.kind {
	case responseAck:
		rs.ack = ackSuccess
		rel := RelevanceLevel(LevelAckOnly)
		if rs.analysis.expectData == expectDataWithAck {
			rs.data = dataReceived
			rel = LevelSoleResponse
		}
		return Correlation{
			Ack:          AckAck,
			InResponseTo: rs.msg,
			Relevance:    rel,
		}
	case responseNak:
		rs.ack = ackFailed
		rs.ackError = ra.ackError
		return Correlation{
			Ack:          AckNak,
			NakError:     ra.ackError,
			InResponseTo: rs.msg,
			Relevance:    LevelAckOnly,
		}
	case responseWait:
		rs.ack = ackWaitMore
		return Correlation{
			Ack:          AckOther,
			InResponseTo: rs.msg,
			Relevance:    LevelAckOnly,
		}
	}
	return Correlation{Relevance: LevelNotResponse}
}

func (c *Correlator) correlateData(tag gpsprot.Tag, data string, confirmed bool) Correlation {
	var matches []*requestState
	for i := range c.requests {
		rs := &c.requests[i]
		if rs.data != dataWait &&
			!(rs.data == dataReceived && rs.analysis.expectData == expectDataMultiple) {
			continue
		}
		a := &rs.analysis
		if !a.dataTag.IsZero() && a.dataTag != tag {
			continue
		}
		if a.dataMatch != nil && !a.dataMatch(data) {
			continue
		}
		matches = append(matches, rs)
	}
	if len(matches) == 0 {
		if confirmed {
			return Correlation{Relevance: LevelNotResponse}
		}
		return Correlation{Relevance: LevelNotResponse}
	}
	if len(matches) > 1 {
		return Correlation{Relevance: LevelAmbigResponse}
	}
	rs := matches[0]
	switch rs.analysis.expectData {
	case expectDataSingle:
		rs.data = dataReceived
		return Correlation{Relevance: LevelSoleResponse}
	case expectDataMultiple:
		rs.data = dataReceived
		return Correlation{Relevance: LevelMultiResponse}
	case expectDataAmbig:
		return Correlation{Relevance: LevelMaybeResponse}
	case expectDataUnknown:
		return Correlation{Relevance: LevelMaybeResponse}
	}
	return Correlation{Relevance: LevelNotResponse}
}

// CanAcceptMore returns true when some requests could still receive
// ack or data responses.
func (c *Correlator) CanAcceptMore() bool {
	for i := range c.requests {
		if !c.requestComplete(&c.requests[i]) {
			return true
		}
	}
	return false
}

func (c *Correlator) requestComplete(rs *requestState) bool {
	a := &rs.analysis
	switch a.expectData {
	case expectDataNone:
		switch a.expectAck {
		case ExpectAckOrNak:
			return rs.ack == ackSuccess || rs.ack == ackFailed
		case ExpectAckNakOnly:
			return rs.ack == ackFailed
		case ExpectAckNone:
			return true
		}
	case expectDataUnknown:
		return false
	case expectDataWithAck:
		return rs.ack == ackSuccess || rs.ack == ackFailed
	case expectDataAmbig:
		if a.expectAck == ExpectAckOrNak || a.expectAck == ExpectAckNakOnly {
			return rs.ack == ackFailed
		}
		return false
	case expectDataSingle:
		switch a.expectAck {
		case ExpectAckOrNak:
			return (rs.ack == ackSuccess && rs.data == dataReceived) || rs.ack == ackFailed
		case ExpectAckNakOnly:
			return rs.ack == ackFailed || rs.data == dataReceived
		case ExpectAckNone:
			return rs.data == dataReceived
		}
	case expectDataMultiple:
		if a.expectAck == ExpectAckOrNak || a.expectAck == ExpectAckNakOnly {
			return rs.ack == ackFailed
		}
		return false
	}
	return false
}

// Missing returns requests whose firm expectations were not met.
// missingAck: requests with ExpectAckOrNak where no ack/nak was received.
// missingData: requests where data was firmly expected but none arrived.
func (c *Correlator) Missing() (missingAck, missingData []*RawMsg) {
	for i := range c.requests {
		rs := &c.requests[i]
		a := &rs.analysis
		if a.expectAck == ExpectAckOrNak && (rs.ack == ackWait || rs.ack == ackWaitMore) {
			missingAck = append(missingAck, rs.msg)
		}
		if rs.ack == ackFailed {
			continue // NAK means complete, no data expected
		}
		switch a.expectData {
		case expectDataSingle, expectDataWithAck, expectDataMultiple:
			if rs.data == dataWait {
				missingData = append(missingData, rs.msg)
			}
		}
	}
	return
}
