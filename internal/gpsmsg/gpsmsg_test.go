package gpsmsg

import (
	"testing"
)

func TestGNSSSet(t *testing.T) {
	// Create a new GNSSSet
	set := GNSSFlag(GPS, GLO)

	// Check that Contains works properly for values in the set
	if !set.Contains(GPS) {
		t.Errorf("expected set to contain GPS")
	}
	if !set.Contains(GLO) {
		t.Errorf("expected set to contain GLONASS")
	}

	// Check that Contains works properly for values not in the set
	if set.Contains(BDS) {
		t.Errorf("expected set to not contain BeiDou")
	}
	if set.Contains(GAL) {
		t.Errorf("expected set to not contain Galileo")
	}

	// Check that Contains works properly for the 0 value
	if set.Contains(0) {
		t.Errorf("expected set to not contain 0")
	}

	// Check that Items returns the correct values
	items := set.Items()
	if len(items) != 2 {
		t.Errorf("expected Items to return 2 items, got %d", len(items))
	}
	if items[0] != GPS {
		t.Errorf("expected first item to be GPS, got %v", items[0])
	}
	if items[1] != GLO {
		t.Errorf("expected second item to be GLONASS, got %v", items[1])
	}
}

func TestGNSSSetMarshalJSON(t *testing.T) {
	gnssSet := GNSSFlag(GPS) | GNSSFlag(GAL)

	marshaledJSON, err := gnssSet.MarshalJSON()

	if err != nil {
		t.Errorf("MarshalJSON returned error: %v", err)
	}

	expectedJSON := `["GPS","GAL"]`
	if string(marshaledJSON) != expectedJSON {
		t.Errorf("Expected marshaled JSON to be %v, got %v", expectedJSON, string(marshaledJSON))
	}
}
