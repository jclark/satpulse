package logobs

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestFixType(t *testing.T) {
	tests := []struct {
		name  string
		level gpsprot.FixLevel
		dim   gpsprot.FixDim
		want  string
	}{
		// Both zero (not provided)
		{"zero values", 0, 0, ""},
		// FixLevelNone always returns "none"
		{"none", gpsprot.FixLevelNone, 0, "none"},
		{"none 2d", gpsprot.FixLevelNone, gpsprot.FixDim2D, "none"},
		{"none 3d", gpsprot.FixLevelNone, gpsprot.FixDim3D, "none"},
		{"none time only", gpsprot.FixLevelNone, gpsprot.FixDimTimeOnly, "none"},
		{"none velocity only", gpsprot.FixLevelNone, gpsprot.FixDimVelocityOnly, "none"},
		// Non-position dim returns ""
		{"time only", 0, gpsprot.FixDimTimeOnly, ""},
		{"velocity only", 0, gpsprot.FixDimVelocityOnly, ""},
		{"code time only", gpsprot.FixLevelCode, gpsprot.FixDimTimeOnly, ""},
		{"code corrected time only", gpsprot.FixLevelCodeCorrected, gpsprot.FixDimTimeOnly, ""},
		{"carrier fixed time only", gpsprot.FixLevelCarrierFixed, gpsprot.FixDimTimeOnly, ""},
		{"code velocity only", gpsprot.FixLevelCode, gpsprot.FixDimVelocityOnly, ""},
		// Level without dim
		{"not measured no dim", gpsprot.FixLevelNotMeasured, 0, ""},
		{"code no dim", gpsprot.FixLevelCode, 0, ""},
		{"code corrected no dim", gpsprot.FixLevelCodeCorrected, 0, ""},
		// 2D fixes
		{"2d", 0, gpsprot.FixDim2D, "2d"},
		{"code 2d", gpsprot.FixLevelCode, gpsprot.FixDim2D, "2d"},
		{"not measured 2d", gpsprot.FixLevelNotMeasured, gpsprot.FixDim2D, "2d"},
		{"code corrected 2d", gpsprot.FixLevelCodeCorrected, gpsprot.FixDim2D, "2d"},
		// 3D fixes
		{"3d", 0, gpsprot.FixDim3D, "3d"},
		{"code 3d", gpsprot.FixLevelCode, gpsprot.FixDim3D, "3d"},
		{"not measured 3d", gpsprot.FixLevelNotMeasured, gpsprot.FixDim3D, "3d"},
		{"code corrected 3d", gpsprot.FixLevelCodeCorrected, gpsprot.FixDim3D, "dgps"},
		{"carrier float 3d", gpsprot.FixLevelCarrierFloat, gpsprot.FixDim3D, "dgps"},
		{"carrier fixed 3d", gpsprot.FixLevelCarrierFixed, gpsprot.FixDim3D, "dgps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixType(tt.level, tt.dim)
			if got != tt.want {
				t.Errorf("fixType(%v, %v) = %q, want %q", tt.level, tt.dim, got, tt.want)
			}
		})
	}
}
