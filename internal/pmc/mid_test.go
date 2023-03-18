package pmc

import (
	"encoding/binary"
	"testing"
)

func TestSizes(t *testing.T) {
	if sz := binary.Size(DefaultDS{}); sz != 20 {
		t.Fatalf("wrong size for DefaultDataSet struct: got %d, expected 20", sz)
	}
	if sz := binary.Size(CurrentDS{}); sz != 18 {
		t.Fatalf("wrong size for CurrentDataSet struct: got %d, expected 18", sz)
	}
	if sz := binary.Size(ParentDS{}); sz != 32 {
		t.Fatalf("wrong size for ParentDataSet struct: got %d, expected 32", sz)
	}
	if sz := binary.Size(TimePropertiesDS{}); sz != 4 {
		t.Fatalf("wrong size for TimePropertiesDataSet struct: got %d, expected 4", sz)
	}
}
