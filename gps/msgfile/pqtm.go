package msgfile

import (
	"strings"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
)

// pqtmClassifier dispatches PQTM classification to the qtmmsg package.
type pqtmClassifier struct{}

func (pqtmClassifier) classifyRequest(payload string) requestAnalysis {
	rc := qtmmsg.ClassifyRequest(payload)
	a := requestAnalysis{
		ackTag:       gpsreg.TagNMEA,
		ackCorrelate: rc.Sentence,
		dataTag:      gpsreg.TagNMEA,
	}
	switch rc.Kind {
	case qtmmsg.RequestCommand:
		a.expectAck = ExpectAckOrNak
		a.expectData = expectDataNone
	case qtmmsg.RequestQuery:
		a.expectAck = ExpectAckOrNak
		a.expectData = expectDataWithAck
	case qtmmsg.RequestVerno:
		a.expectAck = ExpectAckNakOnly
		a.expectData = expectDataSingle
		a.dataMatch = func(data string) bool {
			p := nmeaPayload(data)
			name, _, _ := strings.Cut(p, ",")
			return name == rc.Sentence
		}
	}
	return a
}

func (pqtmClassifier) classifyResponse(payload string) responseAnalysis {
	rc := qtmmsg.ClassifyResponse(payload)
	switch rc.Kind {
	case qtmmsg.ResponseOK, qtmmsg.ResponseOKData:
		return responseAnalysis{kind: responseAck, ackCorrelate: rc.Sentence}
	case qtmmsg.ResponseError:
		return responseAnalysis{kind: responseNak, ackCorrelate: rc.Sentence, ackError: rc.Error}
	case qtmmsg.ResponseData:
		return responseAnalysis{kind: responseData}
	}
	return responseAnalysis{kind: responseMaybeData}
}
