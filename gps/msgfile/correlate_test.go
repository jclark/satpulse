package msgfile

import (
	"path/filepath"
	"testing"
)

// event is a step in a correlator test scenario.
type event interface {
	run(t *testing.T, tc *testContext)
}

type testContext struct {
	cor  *Correlator
	msgs []RawMsg
	cur  int         // next message index for sendEvent
	last Correlation // most recent CorrelatePacket result
}

// sendEvent calls NotifyMsgSent with the next message.
type sendEvent struct{}

func (sendEvent) run(t *testing.T, tc *testContext) {
	t.Helper()
	if tc.cur >= len(tc.msgs) {
		t.Fatal("sendEvent: no more messages")
	}
	tc.cor.NotifyMsgSent(tc.msgs[tc.cur])
	tc.cur++
}

// readyToSend asserts ReadyToSend on the next unsent message.
type readyToSend struct {
	want bool
}

func (e readyToSend) run(t *testing.T, tc *testContext) {
	t.Helper()
	if tc.cur >= len(tc.msgs) {
		t.Fatal("readyToSend: no more messages")
	}
	got := tc.cor.ReadyToSend(tc.msgs[tc.cur])
	if got != e.want {
		t.Errorf("ReadyToSend = %v, want %v", got, e.want)
	}
}

// expect asserts against the most recent Correlation.
type expect struct {
	ack       AckKind
	relevance RelevanceLevel
	msgIndex  *int // index into resolved messages; nil = InResponseTo is nil
}

func (e expect) run(t *testing.T, tc *testContext) {
	t.Helper()
	cor := tc.last
	if e.ack != AckNone && cor.Ack != e.ack {
		t.Errorf("Ack = %d, want %d", cor.Ack, e.ack)
	}
	if e.relevance != 0 && cor.Relevance != e.relevance {
		t.Errorf("Relevance = %d, want %d", cor.Relevance, e.relevance)
	}
	if e.msgIndex != nil {
		if cor.InResponseTo == nil {
			t.Error("InResponseTo is nil, want non-nil")
		} else {
			want := tc.msgs[*e.msgIndex].MsgID()
			got := cor.InResponseTo.MsgID()
			if got != want {
				t.Errorf("InResponseTo = %+v, want %+v", got, want)
			}
		}
	} else if e.ack == AckNone && cor.InResponseTo != nil {
		t.Error("InResponseTo is non-nil, want nil")
	}
}

// checkDone asserts CanAcceptMore.
type checkDone struct {
	canAcceptMore bool
}

func (e checkDone) run(t *testing.T, tc *testContext) {
	t.Helper()
	got := tc.cor.CanAcceptMore()
	if got != e.canAcceptMore {
		t.Errorf("CanAcceptMore = %v, want %v", got, e.canAcceptMore)
	}
}

// checkMissing asserts Missing.
type checkMissing struct {
	ack  []int // indices into resolved messages
	data []int
}

func (e checkMissing) run(t *testing.T, tc *testContext) {
	t.Helper()
	mAck, mData := tc.cor.Missing()
	assertMissingList(t, "ack", mAck, e.ack, tc.msgs)
	assertMissingList(t, "data", mData, e.data, tc.msgs)
}

func assertMissingList(t *testing.T, name string, got []*RawMsg, wantIdx []int, msgs []RawMsg) {
	t.Helper()
	if len(got) != len(wantIdx) {
		t.Errorf("Missing %s: got %d, want %d", name, len(got), len(wantIdx))
		return
	}
	for i, rm := range got {
		want := msgs[wantIdx[i]].MsgID()
		gotID := rm.MsgID()
		if gotID != want {
			t.Errorf("Missing %s[%d] = %+v, want %+v", name, i, gotID, want)
		}
	}
}

type correlatorTest struct {
	name   string
	tags   []string
	events []event
}

func loadTestFile(t *testing.T, file string) *Parsed {
	t.Helper()
	path := filepath.Join("testdata", file)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}
	return mf
}

func runCorrelatorTests(t *testing.T, file string, tests []correlatorTest) {
	t.Helper()
	mf := loadTestFile(t, file)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs, err := mf.TaggedMsgs(tc.tags)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := ToRaw(msgs, false)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &testContext{
				cor:  NewCorrelator(),
				msgs: raw,
			}
			for _, ev := range tc.events {
				ev.run(t, ctx)
			}
		})
	}
}

func intptr(v int) *int { return &v }
