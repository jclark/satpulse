package airmsg

import "testing"

func TestClassifyResponse(t *testing.T) {
	tests := []struct {
		name    string
		recv    string
		wantKind ResponseKind
		wantID  string
		wantErr string
	}{
		{
			name:     "ack success",
			recv:     "PAIR001,072,0",
			wantKind: ResponseOK,
			wantID:   "072",
		},
		{
			name:     "processing wait",
			recv:     "PAIR001,072,1",
			wantKind: ResponseWait,
			wantID:   "072",
		},
		{
			name:     "ack sending failed",
			recv:     "PAIR001,072,2",
			wantKind: ResponseError,
			wantID:   "072",
			wantErr:  "sending failed",
		},
		{
			name:     "ack command not supported",
			recv:     "PAIR001,072,3",
			wantKind: ResponseError,
			wantID:   "072",
			wantErr:  "command not supported",
		},
		{
			name:     "ack parameter error",
			recv:     "PAIR001,072,4",
			wantKind: ResponseError,
			wantID:   "072",
			wantErr:  "parameter error",
		},
		{
			name:     "ack service busy",
			recv:     "PAIR001,072,5",
			wantKind: ResponseError,
			wantID:   "072",
			wantErr:  "service busy",
		},
		{
			name:     "ack unknown code",
			recv:     "PAIR001,072,99",
			wantKind: ResponseError,
			wantID:   "072",
			wantErr:  "unknown error 99",
		},
		{
			name:     "ack missing result code",
			recv:     "PAIR001,072",
			wantKind: ResponseNotPAIR,
		},
		{
			name:     "ack non-numeric result",
			recv:     "PAIR001,072,abc",
			wantKind: ResponseNotPAIR,
		},
		{
			name:     "ack negative result",
			recv:     "PAIR001,072,-1",
			wantKind: ResponseNotPAIR,
		},
		{
			name:     "data response",
			recv:     "PAIR073,5",
			wantKind: ResponseData,
			wantID:   "073",
		},
		{
			name:     "data response no fields",
			recv:     "PAIR073",
			wantKind: ResponseData,
			wantID:   "073",
		},
		{
			name:     "PAIR001 no fields",
			recv:     "PAIR001",
			wantKind: ResponseNotPAIR,
		},
		{
			name:     "unrelated message",
			recv:     "PQTMCFGPPS,OK",
			wantKind: ResponseNotPAIR,
		},
		{
			name:     "not PAIR at all",
			recv:     "GPRMC,123456.00,A",
			wantKind: ResponseNotPAIR,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc := ClassifyResponse(tc.recv)
			if rc.Kind != tc.wantKind {
				t.Errorf("kind: got %v, want %v", rc.Kind, tc.wantKind)
			}
			if tc.wantID != "" && rc.CommandID != tc.wantID {
				t.Errorf("commandID: got %q, want %q", rc.CommandID, tc.wantID)
			}
			if tc.wantErr != "" && rc.Error != tc.wantErr {
				t.Errorf("error: got %q, want %q", rc.Error, tc.wantErr)
			}
		})
	}
}

func TestClassifyRequest(t *testing.T) {
	tests := []struct {
		name     string
		sent     string
		wantKind RequestKind
		wantID   string
	}{
		{
			name:     "even ID = command",
			sent:     "PAIR062,1,1",
			wantKind: RequestCommand,
			wantID:   "062",
		},
		{
			name:     "odd ID = query",
			sent:     "PAIR073",
			wantKind: RequestQuery,
			wantID:   "073",
		},
		{
			name:     "exception: 003 is query despite odd",
			sent:     "PAIR003",
			wantKind: RequestCommand,
			wantID:   "003",
		},
		{
			name:     "exception: 008 is query despite even",
			sent:     "PAIR008",
			wantKind: RequestQuery,
			wantID:   "008",
		},
		{
			name:     "exception: 020 is query despite even",
			sent:     "PAIR020",
			wantKind: RequestQuery,
			wantID:   "020",
		},
		{
			name:     "exception: 412 is query despite even",
			sent:     "PAIR412",
			wantKind: RequestQuery,
			wantID:   "412",
		},
		{
			name:     "normal even command 410",
			sent:     "PAIR410,1",
			wantKind: RequestCommand,
			wantID:   "410",
		},
		{
			name:     "normal odd query 411",
			sent:     "PAIR411",
			wantKind: RequestQuery,
			wantID:   "411",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc := ClassifyRequest(tc.sent)
			if rc.Kind != tc.wantKind {
				t.Errorf("kind: got %v, want %v", rc.Kind, tc.wantKind)
			}
			if rc.CommandID != tc.wantID {
				t.Errorf("commandID: got %q, want %q", rc.CommandID, tc.wantID)
			}
		})
	}
}
