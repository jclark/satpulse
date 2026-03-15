package airmsg

import "testing"

func TestCheckResponse(t *testing.T) {
	tests := []struct {
		name     string
		sent     string
		recv     string
		wantKind ResponseKind
		wantMsg  string
	}{
		{
			name:     "ack success",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,0",
			wantKind: ResponseOK,
		},
		{
			name:     "processing wait",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,1",
			wantKind: ResponseWait,
		},
		{
			name:     "ack sending failed",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,2",
			wantKind: ResponseError,
			wantMsg:  "sending failed",
		},
		{
			name:     "ack command not supported",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,3",
			wantKind: ResponseError,
			wantMsg:  "command not supported",
		},
		{
			name:     "ack parameter error",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,4",
			wantKind: ResponseError,
			wantMsg:  "parameter error",
		},
		{
			name:     "ack service busy",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,5",
			wantKind: ResponseError,
			wantMsg:  "service busy",
		},
		{
			name:     "ack unknown code",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,99",
			wantKind: ResponseError,
			wantMsg:  "unknown error 99",
		},
		{
			name:     "ack missing result code",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072",
			wantKind: ResponseInvalid,
		},
		{
			name:     "ack empty result code",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,",
			wantKind: ResponseInvalid,
		},
		{
			name:     "ack non-numeric result",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,abc",
			wantKind: ResponseInvalid,
		},
		{
			name:     "ack negative result",
			sent:     "PAIR072,5",
			recv:     "PAIR001,072,-1",
			wantKind: ResponseInvalid,
		},
		{
			name:     "data response",
			sent:     "PAIR073",
			recv:     "PAIR073,5",
			wantKind: ResponseData,
		},
		{
			name:     "data response no fields",
			sent:     "PAIR073",
			recv:     "PAIR073",
			wantKind: ResponseData,
		},
		{
			name:     "command ID mismatch in ack",
			sent:     "PAIR072,5",
			recv:     "PAIR001,073,0",
			wantKind: NotResponse,
		},
		{
			name:     "different command ID in data",
			sent:     "PAIR072,5",
			recv:     "PAIR073,5",
			wantKind: NotResponse,
		},
		{
			name:     "PAIR001 no fields",
			sent:     "PAIR072,5",
			recv:     "PAIR001",
			wantKind: NotResponse,
		},
		{
			name:     "unrelated message",
			sent:     "PAIR072,5",
			recv:     "PQTMCFGPPS,OK",
			wantKind: NotResponse,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, msg := CheckResponse(tc.sent, tc.recv)
			if kind != tc.wantKind {
				t.Errorf("kind: got %v, want %v", kind, tc.wantKind)
			}
			if kind == ResponseError && msg != tc.wantMsg {
				t.Errorf("msg: got %q, want %q", msg, tc.wantMsg)
			}
		})
	}
}
