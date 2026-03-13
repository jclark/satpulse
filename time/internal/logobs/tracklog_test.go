package logobs

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
)

func TestFixType(t *testing.T) {
	tests := []struct {
		name  string
		level gpsprot.FixLevel
		dim   gpsprot.SolutionDim
		corr  gpsprot.CorrKind
		want  string
	}{
		// Both zero (not provided)
		{"zero values", 0, 0, 0, ""},
		// FixLevelNone always returns "none"
		{"none", gpsprot.FixLevelNone, 0, 0, "none"},
		{"none 2d", gpsprot.FixLevelNone, gpsprot.SolutionDim2D, 0, "none"},
		{"none 3d", gpsprot.FixLevelNone, gpsprot.SolutionDim3D, 0, "none"},
		{"none time only", gpsprot.FixLevelNone, gpsprot.SolutionDimTimeOnly, 0, "none"},
		// Non-position dim returns ""
		{"time only", 0, gpsprot.SolutionDimTimeOnly, 0, ""},
		{"doppler only", gpsprot.FixLevelDoppler, 0, 0, ""},
		{"code time only", gpsprot.FixLevelCode, gpsprot.SolutionDimTimeOnly, 0, ""},
		{"code corrected time only", gpsprot.FixLevelCode, gpsprot.SolutionDimTimeOnly, gpsprot.CorrUsed, ""},
		{"carrier fixed time only", gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDimTimeOnly, 0, ""},
		// Level without dim
		{"not measured no dim", gpsprot.FixLevelNotMeasured, 0, 0, ""},
		{"code no dim", gpsprot.FixLevelCode, 0, 0, ""},
		{"code corrected no dim", gpsprot.FixLevelCode, 0, gpsprot.CorrUsed, ""},
		// 2D fixes
		{"2d", 0, gpsprot.SolutionDim2D, 0, "2d"},
		{"code 2d", gpsprot.FixLevelCode, gpsprot.SolutionDim2D, 0, "2d"},
		{"not measured 2d", gpsprot.FixLevelNotMeasured, gpsprot.SolutionDim2D, 0, "2d"},
		{"code corrected 2d", gpsprot.FixLevelCode, gpsprot.SolutionDim2D, gpsprot.CorrUsed, "2d"},
		// 3D fixes
		{"3d", 0, gpsprot.SolutionDim3D, 0, "3d"},
		{"code 3d", gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, "3d"},
		{"not measured 3d", gpsprot.FixLevelNotMeasured, gpsprot.SolutionDim3D, 0, "3d"},
		{"code corrected 3d", gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrUsed, "dgps"},
		{"carrier float 3d", gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), "dgps"},
		{"carrier fixed 3d", gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrOSR.Expand(), "dgps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixType(tt.level, tt.dim, tt.corr)
			if got != tt.want {
				t.Errorf("fixType(%v, %v, %v) = %q, want %q", tt.level, tt.dim, tt.corr, got, tt.want)
			}
		})
	}
}
