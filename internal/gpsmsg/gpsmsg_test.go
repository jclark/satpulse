package gpsmsg

import (
    "testing"
)

func TestMajorGNSSSet(t *testing.T) {
    // Create a new MajorGNSSSet
    set := MajorGNSSSet(0)

    // Add some MajorGNSS values to the set
    set |= MajorGNSSFlag(GPS)
    set |= MajorGNSSFlag(GLONASS)

    // Check that Contains works properly for values in the set
    if !set.Contains(GPS) {
        t.Errorf("expected set to contain GPS")
    }
    if !set.Contains(GLONASS) {
        t.Errorf("expected set to contain GLONASS")
    }

    // Check that Contains works properly for values not in the set
    if set.Contains(BeiDou) {
        t.Errorf("expected set to not contain BeiDou")
    }
    if set.Contains(Galileo) {
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
    if items[1] != GLONASS {
        t.Errorf("expected second item to be GLONASS, got %v", items[1])
    }
}