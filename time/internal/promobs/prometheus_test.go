package promobs

import "testing"

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		// FixLevel values
		{"none", "none"},
		{"notMeasured", "not_measured"},
		{"doppler", "doppler"},
		{"code", "code"},
		{"codeCorrected", "code_corrected"},
		{"carrierFloat", "carrier_float"},
		{"carrierFixed", "carrier_fixed"},
		// SolutionDim values
		{"2D", "2d"},
		{"3D", "3d"},
		{"timeOnly", "time_only"},
		// CorrKind values
		{"used", "used"},
		{"OSR", "osr"},
		{"SSR", "ssr"},
		{"RTCM", "rtcm"},
		{"partialDualFreq", "partial_dual_freq"},
		{"fullDualFreq", "full_dual_freq"},
		{"SBAS", "sbas"},
		{"CLAS", "clas"},
		{"SPARTN", "spartn"},
		{"PPP", "ppp"},
		{"PPP-RTK", "ppp_rtk"},
		{"PPPConverging", "ppp_converging"},
		{"PPPConverged", "ppp_converged"},
		{"PPP-HAS", "ppp_has"},
		{"PPP-MDC", "ppp_mdc"},
		{"PPP-B2b", "ppp_b2b"},
	}
	for _, tt := range tests {
		got := camelToSnake(tt.in)
		if got != tt.want {
			t.Errorf("camelToSnake(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
