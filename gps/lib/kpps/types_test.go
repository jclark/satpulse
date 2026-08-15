package kpps

import "testing"

func TestCaptureModes(t *testing.T) {
	if CaptureAssert != 0x01 {
		t.Errorf("CaptureAssert = %#x, want 0x01", CaptureAssert)
	}
	if CaptureClear != 0x02 {
		t.Errorf("CaptureClear = %#x, want 0x02", CaptureClear)
	}
}
