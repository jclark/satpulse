package msgfile

import (
	"strings"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/airmsg"
)

// pairClassifier dispatches PAIR classification to the airmsg package.
type pairClassifier struct{}

func (pairClassifier) classifyRequest(payload string) requestAnalysis {
	rc := airmsg.ClassifyRequest(payload)
	a := requestAnalysis{
		ackTag:       gpsreg.TagNMEA,
		ackCorrelate: rc.CommandID,
		expectAck:    ExpectAckOrNak,
		dataTag:      gpsreg.TagNMEA,
	}
	switch rc.Kind {
	case airmsg.RequestCommand:
		a.expectData = expectDataNone
	case airmsg.RequestQuery:
		a.expectData = expectDataSingle
		sentName := "PAIR" + rc.CommandID
		a.dataMatch = func(data string) bool {
			p := nmeaPayload(data)
			name, _, _ := strings.Cut(p, ",")
			return name == sentName
		}
	}
	return a
}

func (pairClassifier) classifyResponse(payload string) responseAnalysis {
	rc := airmsg.ClassifyResponse(payload)
	switch rc.Kind {
	case airmsg.ResponseOK:
		return responseAnalysis{kind: responseAck, ackCorrelate: rc.CommandID}
	case airmsg.ResponseWait:
		return responseAnalysis{kind: responseWait, ackCorrelate: rc.CommandID}
	case airmsg.ResponseError:
		return responseAnalysis{kind: responseNak, ackCorrelate: rc.CommandID, ackError: rc.Error}
	case airmsg.ResponseData:
		return responseAnalysis{kind: responseData}
	}
	return responseAnalysis{kind: responseMaybeData}
}
