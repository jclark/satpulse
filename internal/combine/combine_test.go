package combine

import (
	"testing"

	"github.com/jclark/satpulse/internal/ptime"
)

func TestSearch(t *testing.T) {
	secStates := []*secMsgState{
		{sec: 100},
		{sec: 200},
		{sec: 300},
		{sec: 400},
	}

	testCases := []struct {
		sec      ptime.Time
		expected int
	}{
		{100, 0},
		{50, 0},
		{500, 4},
		{400, 3},
	}

	for _, tc := range testCases {
		i := search(secStates, tc.sec)
		if i != tc.expected {
			t.Errorf("search(secStates, %v) = %v, want %v", tc.sec, i, tc.expected)
		}
	}
}
