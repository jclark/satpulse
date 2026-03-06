package qtmmsg

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
			name:     "write ack OK",
			sent:     "PQTMCFGPPS,W,1,1,100000,1000,0,0",
			recv:     "PQTMCFGPPS,OK",
			wantKind: ResponseOK,
		},
		{
			name:     "query response OK with data",
			sent:     "PQTMCFGPPS,R,1",
			recv:     "PQTMCFGPPS,OK,1,1,100000,1000,0,0",
			wantKind: ResponseData,
		},
		{
			name:     "error invalid parameters",
			sent:     "PQTMCFGPPS,W,1",
			recv:     "PQTMCFGPPS,ERROR,1",
			wantKind: ResponseError,
			wantMsg:  "invalid parameters",
		},
		{
			name:     "error execution failed",
			sent:     "PQTMCFGPPS,W,1",
			recv:     "PQTMCFGPPS,ERROR,2",
			wantKind: ResponseError,
			wantMsg:  "execution failed",
		},
		{
			name:     "error unsupported command",
			sent:     "PQTMCFGPPS,W,1",
			recv:     "PQTMCFGPPS,ERROR,3",
			wantKind: ResponseError,
			wantMsg:  "unsupported command",
		},
		{
			name:     "error unknown code",
			sent:     "PQTMCFGPPS,W,1",
			recv:     "PQTMCFGPPS,ERROR,99",
			wantKind: ResponseError,
			wantMsg:  "99",
		},
		{
			name:     "error no code",
			sent:     "PQTMCFGPPS,W,1",
			recv:     "PQTMCFGPPS,ERROR",
			wantKind: ResponseError,
			wantMsg:  "unknown error",
		},
		{
			name:     "PQTMVERNO data-only response",
			sent:     "PQTMVERNO",
			recv:     "PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07",
			wantKind: ResponseData,
		},
		{
			name:     "simple command bare ack",
			sent:     "PQTMSAVEPAR",
			recv:     "PQTMSAVEPAR,OK",
			wantKind: ResponseOK,
		},
		{
			name:     "name mismatch",
			sent:     "PQTMCFGPPS,W,1",
			recv:     "PQTMCFGMSGRATE,OK",
			wantKind: NotResponse,
		},
		{
			name:     "data reply without OK",
			sent:     "PQTMCFGPPS,R,1",
			recv:     "PQTMCFGPPS,1,1,100000,1000,0,0",
			wantKind: ResponseData,
		},
		{
			name:     "recv has no fields after name",
			sent:     "PQTMSAVEPAR",
			recv:     "PQTMSAVEPAR",
			wantKind: ResponseData,
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
