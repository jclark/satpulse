package promobs

import (
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestCamelToSnake(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		// FixLevel values
		{"none", "none"},
		{"notMeasured", "not_measured"},
		{"doppler", "doppler"},
		{"code", "code"},
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

func TestCorReportRTCMInputMetrics(t *testing.T) {
	obs := New(100)
	mf := scrape(t, obs)
	if countMetrics(mf, "satpulse_rtcm_input_ref_station_id") != 0 {
		t.Fatal("satpulse_rtcm_input_ref_station_id present before first reference station ID")
	}
	tRead := time.Unix(0, 0)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source:        gpsprot.CorReportSourcePull,
		Tag:           rtcmTag,
		MsgID:         "1077",
		NBytes:        opt.Make(100),
		ChecksumOK:    opt.Make(true),
		FinalFragment: opt.Make(false),
		RTCMRefBaseID: opt.Make(uint16(12)),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source:        gpsprot.CorReportSourcePull,
		Tag:           rtcmTag,
		MsgID:         "1077",
		NBytes:        opt.Make(120),
		FinalFragment: opt.Make(true),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source: gpsprot.CorReportSourcePull,
		Tag:    rtcmTag,
		MsgID:  "1005",
		NBytes: opt.Make(50),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source:        gpsprot.CorReportSourcePull,
		Tag:           rtcmTag,
		NBytes:        opt.Make(30),
		RTCMRefBaseID: opt.Make(uint16(14)),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source:        gpsprot.CorReportSourceReceiver,
		Tag:           rtcmTag,
		MsgID:         "1077",
		Used:          opt.Make(true),
		RTCMRefBaseID: opt.Make(uint16(21)),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source: gpsprot.CorReportSourceReceiver,
		Tag:    rtcmTag,
		MsgID:  "1077",
		Used:   opt.Make(false),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source: gpsprot.CorReportSourceReceiver,
		Tag:    rtcmTag,
		MsgID:  "1005",
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source:        gpsprot.CorReportSourcePull,
		Tag:           "SPARTN",
		MsgID:         "999",
		NBytes:        opt.Make(1000),
		RTCMRefBaseID: opt.Make(uint16(99)),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source:        gpsprot.CorReportSource(42),
		Tag:           rtcmTag,
		MsgID:         "1087",
		NBytes:        opt.Make(1000),
		RTCMRefBaseID: opt.Make(uint16(88)),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source:        gpsprot.CorReportSourcePull,
		Tag:           rtcmTag,
		MsgID:         "9999",
		NBytes:        opt.Make(1000),
		ChecksumOK:    opt.Make(false),
		RTCMRefBaseID: opt.Make(uint16(77)),
	}, tRead)
	obs.CorReport(&gpsprot.CorReportMsg{
		Source:     gpsprot.CorReportSourceReceiver,
		Tag:        rtcmTag,
		MsgID:      "1005",
		ChecksumOK: opt.Make(false),
		Used:       opt.Make(true),
	}, tRead)

	mf = scrape(t, obs)
	assertCounterLabel(t, mf, "satpulse_rtcm_input_packets_observed_total", "msg_id", "1077", 2)
	assertCounterLabel(t, mf, "satpulse_rtcm_input_packets_observed_total", "msg_id", "1005", 1)
	assertCounterLabel(t, mf, "satpulse_rtcm_input_messages_total", "msg_id", "1077", 1)
	assertCounterLabel(t, mf, "satpulse_rtcm_input_messages_total", "msg_id", "1005", 1)
	assertCounter(t, mf, "satpulse_rtcm_input_bytes_total", 300)
	assertCounter(t, mf, "satpulse_rtcm_input_errors_observed_total", 1)
	assertCounterLabel(t, mf, "satpulse_rtcm_input_packets_reported_total", "msg_id", "1077", 2)
	assertCounterLabel(t, mf, "satpulse_rtcm_input_packets_reported_total", "msg_id", "1005", 1)
	assertCounterLabel(t, mf, "satpulse_rtcm_input_packets_used_total", "msg_id", "1077", 1)
	assertCounterLabel(t, mf, "satpulse_rtcm_input_packets_unused_total", "msg_id", "1077", 1)
	assertCounter(t, mf, "satpulse_rtcm_input_errors_reported_total", 1)
	assertGauge(t, mf, "satpulse_rtcm_input_ref_station_id", 21)
	assertNoCounterLabel(t, mf, "satpulse_rtcm_input_packets_observed_total", "msg_id", "1087")
	assertNoCounterLabel(t, mf, "satpulse_rtcm_input_packets_observed_total", "msg_id", "999")
	assertNoCounterLabel(t, mf, "satpulse_rtcm_input_packets_observed_total", "msg_id", "9999")
	assertNoCounterLabel(t, mf, "satpulse_rtcm_input_packets_used_total", "msg_id", "1005")
	assertNoCounterLabel(t, mf, "satpulse_rtcm_input_packets_unused_total", "msg_id", "1005")
}

func assertNoCounterLabel(t *testing.T, mf map[string]*dto.MetricFamily, name, labelName, labelVal string) {
	t.Helper()
	if m := findMetric(mf, name, labelName, labelVal); m != nil {
		t.Errorf("%s{%s=%q}: found value %g, want absent", name, labelName, labelVal, m.GetCounter().GetValue())
	}
}
