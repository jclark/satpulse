package bin

import (
	"testing"

	"golang.org/x/exp/slices"
)

func TestInf(t *testing.T) {
	bytes := ([]byte)("hello")
	m := InfDebug{InfText(bytes)}
	p2 := testMsgType1(t, m)
	if !slices.Equal(bytes, ([]byte)(p2.(*InfDebug).InfText)) {
		t.Fatalf("msg inf not roundtripped %v => %v", &m, p2)
	}
}
