package qtmmsg

import "testing"

func TestClassifyResponse(t *testing.T) {
	tests := []struct {
		name     string
		recv     string
		wantKind ResponseKind
		wantSent string
		wantErr  string
	}{
		{
			name:     "write ack OK",
			recv:     "PQTMCFGPPS,OK",
			wantKind: ResponseOK,
			wantSent: "PQTMCFGPPS",
		},
		{
			name:     "query response OK with data",
			recv:     "PQTMCFGPPS,OK,1,1,100000,1000,0,0",
			wantKind: ResponseOKData,
			wantSent: "PQTMCFGPPS",
		},
		{
			name:     "error invalid parameters",
			recv:     "PQTMCFGPPS,ERROR,1",
			wantKind: ResponseError,
			wantSent: "PQTMCFGPPS",
			wantErr:  "invalid parameters",
		},
		{
			name:     "error execution failed",
			recv:     "PQTMCFGPPS,ERROR,2",
			wantKind: ResponseError,
			wantSent: "PQTMCFGPPS",
			wantErr:  "execution failed",
		},
		{
			name:     "error unsupported command",
			recv:     "PQTMCFGPPS,ERROR,3",
			wantKind: ResponseError,
			wantSent: "PQTMCFGPPS",
			wantErr:  "unsupported command",
		},
		{
			name:     "error unknown code",
			recv:     "PQTMCFGPPS,ERROR,99",
			wantKind: ResponseError,
			wantSent: "PQTMCFGPPS",
			wantErr:  "99",
		},
		{
			name:     "error no code",
			recv:     "PQTMCFGPPS,ERROR",
			wantKind: ResponseError,
			wantSent: "PQTMCFGPPS",
			wantErr:  "unknown error",
		},
		{
			name:     "PQTMVERNO data-only response",
			recv:     "PQTMVERNO,LG290P03AANR01A03S,2024/04/30,10:53:07",
			wantKind: ResponseData,
			wantSent: "PQTMVERNO",
		},
		{
			name:     "simple command bare ack",
			recv:     "PQTMSAVEPAR,OK",
			wantKind: ResponseOK,
			wantSent: "PQTMSAVEPAR",
		},
		{
			name:     "not PQTM",
			recv:     "GPRMC,123456.00,A",
			wantKind: ResponseNotPQTM,
		},
		{
			name:     "data reply without OK",
			recv:     "PQTMCFGPPS,1,1,100000,1000,0,0",
			wantKind: ResponseData,
			wantSent: "PQTMCFGPPS",
		},
		{
			name:     "recv has no fields after name",
			recv:     "PQTMSAVEPAR",
			wantKind: ResponseData,
			wantSent: "PQTMSAVEPAR",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc := ClassifyResponse(tc.recv)
			if rc.Kind != tc.wantKind {
				t.Errorf("kind: got %v, want %v", rc.Kind, tc.wantKind)
			}
			if tc.wantSent != "" && rc.Sentence != tc.wantSent {
				t.Errorf("sentence: got %q, want %q", rc.Sentence, tc.wantSent)
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
		wantSent string
	}{
		{
			name:     "write command",
			sent:     "PQTMCFGPPS,W,1,1,100000,1000,0,0",
			wantKind: RequestCommand,
			wantSent: "PQTMCFGPPS",
		},
		{
			name:     "query command",
			sent:     "PQTMCFGPPS,R,1",
			wantKind: RequestQuery,
			wantSent: "PQTMCFGPPS",
		},
		{
			name:     "version query",
			sent:     "PQTMVERNO",
			wantKind: RequestVerno,
			wantSent: "PQTMVERNO",
		},
		{
			name:     "savepar command",
			sent:     "PQTMSAVEPAR",
			wantKind: RequestCommand,
			wantSent: "PQTMSAVEPAR",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc := ClassifyRequest(tc.sent)
			if rc.Kind != tc.wantKind {
				t.Errorf("kind: got %v, want %v", rc.Kind, tc.wantKind)
			}
			if rc.Sentence != tc.wantSent {
				t.Errorf("sentence: got %q, want %q", rc.Sentence, tc.wantSent)
			}
		})
	}
}
