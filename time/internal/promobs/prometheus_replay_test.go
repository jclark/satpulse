package promobs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/time/internal/gpsevent"
	"github.com/jclark/satpulse/time/internal/phcsync"
)

func TestReplay(t *testing.T) {
	obs := New(100)
	var accum gpsprot.PVMsgAccum
	var lastNavEpoch *gpsprot.NavEpochMsg
	var lastSatellites *gpsprot.SatellitesMsg
	nNavEpoch := 0
	nSatellites := 0
	for ev := range readLogEvents(t, "testdata/session.jsonl") {
		tRead := ev.T
		if ev.PosGeo != nil {
			accum.PosGeo(ev.PosGeo, tRead)
		}
		if ev.PosECEF != nil {
			accum.PosECEF(ev.PosECEF, tRead)
		}
		if ev.VelGeo != nil {
			accum.VelGeo(ev.VelGeo, tRead)
		}
		if ev.VelECEF != nil {
			accum.VelECEF(ev.VelECEF, tRead)
		}
		if ev.NavEpoch != nil {
			accum.PVMsgBundle.FillDerived()
			pv := accum.PVMsgBundle
			accum.NavEpoch(ev.NavEpoch, tRead)
			obs.NavEpochPV(ev.NavEpoch, &pv, tRead)
			lastNavEpoch = ev.NavEpoch
			nNavEpoch++
		}
		if ev.Satellites != nil {
			obs.Satellites(ev.Satellites, tRead)
			lastSatellites = ev.Satellites
			nSatellites++
		}
	}
	if nNavEpoch == 0 {
		t.Fatal("no NavEpoch events in testdata")
	}
	if nSatellites == 0 {
		t.Fatal("no Satellites events in testdata")
	}
	t.Logf("replayed %d NavEpoch, %d Satellites events", nNavEpoch, nSatellites)

	// Also exercise Sample() with hand-crafted PHC samples
	obs.Sample(phcsync.Sample{Kind: phcsync.SampleMissing, Mode: phcsync.ModeReset, Freq: 1000})
	obs.Sample(phcsync.Sample{Kind: phcsync.SampleMissing, Mode: phcsync.ModeReset, Freq: 2000})
	obs.Sample(phcsync.Sample{
		Kind: phcsync.SampleOK, Mode: phcsync.ModeTracking, Freq: 500,
		Offset: 50 * time.Nanosecond, FreqDelta: 0.1,
	})
	obs.Sample(phcsync.Sample{
		Kind: phcsync.SampleOutlier, Mode: phcsync.ModeTracking, Freq: 600,
		Offset: 200 * time.Nanosecond,
	})
	obs.Sample(phcsync.Sample{
		Kind: phcsync.SampleOK, Mode: phcsync.ModeTracking, Freq: 700,
		Offset: -30 * time.Nanosecond, FreqDelta: -0.05,
	})
	obs.Sample(phcsync.Sample{Kind: phcsync.SampleMissing, Mode: phcsync.ModeTracking, Freq: 800})

	// Scrape metrics
	mf := scrape(t, obs)

	// -- NavEpoch assertions --
	ne := lastNavEpoch
	// Fix level
	assertGaugeLabel(t, mf, "satpulse_gnss_fix", "level", camelToSnake(ne.FixLevel.String()), 1)
	// Solution dim
	assertGaugeLabel(t, mf, "satpulse_gnss_solution", "dim", camelToSnake(ne.SolutionDim.String()), 1)
	// NumSVUsed
	if ne.NumSVUsed.IsSet() {
		assertGaugeLabel(t, mf, "satpulse_num_satellites", "status", "used", float64(ne.NumSVUsed.Get()))
	}
	// DOP values
	if ne.DOP.Geom.IsSet() {
		assertGaugeLabel(t, mf, "satpulse_dop", "type", "geometric", ne.DOP.Geom.Get())
	}
	if ne.DOP.Hor.IsSet() {
		assertGaugeLabel(t, mf, "satpulse_dop", "type", "horizontal", ne.DOP.Hor.Get())
	}
	if ne.DOP.Vert.IsSet() {
		assertGaugeLabel(t, mf, "satpulse_dop", "type", "vertical", ne.DOP.Vert.Get())
	}
	if ne.DOP.Pos.IsSet() {
		assertGaugeLabel(t, mf, "satpulse_dop", "type", "position", ne.DOP.Pos.Get())
	}
	if ne.DOP.Time.IsSet() {
		assertGaugeLabel(t, mf, "satpulse_dop", "type", "time", ne.DOP.Time.Get())
	}
	// Position accuracy
	if ne.Acc.Hor.IsSet() {
		assertGaugeLabel(t, mf, "satpulse_position_accuracy_meters", "type", "horizontal", ne.Acc.Hor.Get().Meters())
	}
	if ne.Acc.Vert.IsSet() {
		assertGaugeLabel(t, mf, "satpulse_position_accuracy_meters", "type", "vertical", ne.Acc.Vert.Get().Meters())
	}
	// Correction
	if ne.Correction != 0 {
		for _, name := range corrSnakeNames(ne.Correction.Leaves()) {
			assertGaugeLabel(t, mf, "satpulse_gnss_correction_leaf", "kind", name, 1)
		}
	}

	// Constellations used
	if ne.GNSSUsed != 0 {
		for _, g := range ne.GNSSUsed.Items() {
			assertGaugeLabel(t, mf, "satpulse_constellation_used", "gnss", strings.ToLower(g.String()), 1)
		}
	}
	// Bands used
	if ne.BandsUsed != 0 {
		for _, b := range ne.BandsUsed.Items() {
			assertGaugeLabel(t, mf, "satpulse_gnss_band_used", "band", strings.ToLower(b), 1)
		}
	}

	// -- Satellites assertions --
	sat := lastSatellites
	// Count expected active satellites (those with at least one signal with CN0 > 0)
	nActive := 0
	for _, sv := range sat.SVs {
		hasSig := false
		for _, sig := range sv.Signals {
			if sig.CN0 > 0 {
				hasSig = true
				break
			}
		}
		if hasSig {
			nActive++
		}
	}
	// Verify some signal level metrics exist
	nSignalMetrics := countMetrics(mf, "satpulse_satellite_signal_level_dbhz")
	if nSignalMetrics == 0 {
		t.Error("expected signal level metrics after replaying Satellites events")
	}
	t.Logf("active satellites: %d, signal metrics: %d", nActive, nSignalMetrics)
	// Spot-check: first active satellite's first signal
	for _, sv := range sat.SVs {
		for _, sig := range sv.Signals {
			if sig.CN0 == 0 {
				continue
			}
			gnss := sv.ID.GNSS.String()
			svNum := fmt.Sprintf("%02d", sv.ID.Num)
			sigName := signalIDString(sig.ID)
			assertGaugeLabelMulti(t, mf, "satpulse_satellite_signal_level_dbhz",
				map[string]string{"gnss": gnss, "sv": svNum, "signal": sigName},
				float64(sig.CN0))
			goto doneSignalCheck
		}
	}
doneSignalCheck:

	// -- Sample() assertions --
	assertCounterLabel(t, mf, "satpulse_phc_samples_total", "status", "pre_sync", 2)
	assertCounterLabel(t, mf, "satpulse_phc_samples_total", "status", "ok", 2)
	assertCounterLabel(t, mf, "satpulse_phc_samples_total", "status", "outlier", 1)
	assertCounterLabel(t, mf, "satpulse_phc_samples_total", "status", "missing", 1)
	// ModeTracking.InSync() == true, so last sample (Missing in tracking) keeps sync=1
	assertGauge(t, mf, "satpulse_phc_sync_status", 1)
	// Frequency gauge should be last sample's Freq / 1e9
	assertGauge(t, mf, "satpulse_phc_frequency_adjustment", 800.0/1e9)
	// Offset gauge should be the last OK sample's offset: -30ns
	assertGauge(t, mf, "satpulse_phc_offset_seconds", -30e-9)
	// offset_abs_sum = |50e-9| + |(-30e-9)| = 80e-9
	assertCounter(t, mf, "satpulse_phc_offset_abs_sum_seconds_total", 80e-9)
	// offset_sum_squares = (50e-9)^2 + (-30e-9)^2 = 2500e-18 + 900e-18 = 3400e-18
	assertCounter(t, mf, "satpulse_phc_offset_sum_squares_seconds_total", 3400e-18)
	// freq delta count = 2 (two SampleOK)
	assertCounter(t, mf, "satpulse_phc_frequency_delta_count_total", 2)
}

// readLogEvents returns an iterator over LogEvent entries from a JSONL file.
func readLogEvents(t *testing.T, path string) func(yield func(gpsevent.LogEvent) bool) {
	t.Helper()
	return func(yield func(gpsevent.LogEvent) bool) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var ev gpsevent.LogEvent
			if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
				t.Fatalf("unmarshal event: %v\nline: %s", err, sc.Text())
			}
			if !yield(ev) {
				return
			}
		}
		if err := sc.Err(); err != nil {
			t.Fatal(err)
		}
	}
}

// scrape performs an HTTP GET on the observer's /metrics endpoint and parses the response.
func scrape(t *testing.T, obs *PrometheusObserver) map[string]*dto.MetricFamily {
	t.Helper()
	srv := httptest.NewServer(obs.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("scrape returned %d: %s", resp.StatusCode, body)
	}
	parser := expfmt.NewTextParser(model.LegacyValidation)
	mf, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return mf
}

// findMetric finds a metric with the given label value in a MetricFamily.
func findMetric(mf map[string]*dto.MetricFamily, name, labelName, labelVal string) *dto.Metric {
	fam, ok := mf[name]
	if !ok {
		return nil
	}
	for _, m := range fam.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == labelName && lp.GetValue() == labelVal {
				return m
			}
		}
	}
	return nil
}

// findMetricMulti finds a metric matching all given labels.
func findMetricMulti(mf map[string]*dto.MetricFamily, name string, labels map[string]string) *dto.Metric {
	fam, ok := mf[name]
	if !ok {
		return nil
	}
	for _, m := range fam.GetMetric() {
		lm := make(map[string]string)
		for _, lp := range m.GetLabel() {
			lm[lp.GetName()] = lp.GetValue()
		}
		match := true
		for k, v := range labels {
			if lm[k] != v {
				match = false
				break
			}
		}
		if match {
			return m
		}
	}
	return nil
}

// findUnlabelledMetric finds the sole metric in a MetricFamily with no labels.
func findUnlabelledMetric(mf map[string]*dto.MetricFamily, name string) *dto.Metric {
	fam, ok := mf[name]
	if !ok {
		return nil
	}
	for _, m := range fam.GetMetric() {
		if len(m.GetLabel()) == 0 {
			return m
		}
	}
	return nil
}

func assertGaugeLabel(t *testing.T, mf map[string]*dto.MetricFamily, name, labelName, labelVal string, want float64) {
	t.Helper()
	m := findMetric(mf, name, labelName, labelVal)
	if m == nil {
		t.Errorf("%s{%s=%q}: not found", name, labelName, labelVal)
		return
	}
	got := m.GetGauge().GetValue()
	if !approxEqual(got, want) {
		t.Errorf("%s{%s=%q} = %g, want %g", name, labelName, labelVal, got, want)
	}
}

func assertGaugeLabelMulti(t *testing.T, mf map[string]*dto.MetricFamily, name string, labels map[string]string, want float64) {
	t.Helper()
	m := findMetricMulti(mf, name, labels)
	if m == nil {
		t.Errorf("%s{%v}: not found", name, labels)
		return
	}
	got := m.GetGauge().GetValue()
	if !approxEqual(got, want) {
		t.Errorf("%s{%v} = %g, want %g", name, labels, got, want)
	}
}

func assertGauge(t *testing.T, mf map[string]*dto.MetricFamily, name string, want float64) {
	t.Helper()
	m := findUnlabelledMetric(mf, name)
	if m == nil {
		t.Errorf("%s: not found", name)
		return
	}
	got := m.GetGauge().GetValue()
	if !approxEqual(got, want) {
		t.Errorf("%s = %g, want %g", name, got, want)
	}
}

func assertCounter(t *testing.T, mf map[string]*dto.MetricFamily, name string, want float64) {
	t.Helper()
	m := findUnlabelledMetric(mf, name)
	if m == nil {
		t.Errorf("%s: not found", name)
		return
	}
	got := m.GetCounter().GetValue()
	if !approxEqual(got, want) {
		t.Errorf("%s = %g, want %g", name, got, want)
	}
}

func assertCounterLabel(t *testing.T, mf map[string]*dto.MetricFamily, name, labelName, labelVal string, want float64) {
	t.Helper()
	m := findMetric(mf, name, labelName, labelVal)
	if m == nil {
		t.Errorf("%s{%s=%q}: not found", name, labelName, labelVal)
		return
	}
	got := m.GetCounter().GetValue()
	if !approxEqual(got, want) {
		t.Errorf("%s{%s=%q} = %g, want %g", name, labelName, labelVal, got, want)
	}
}

func countMetrics(mf map[string]*dto.MetricFamily, name string) int {
	fam, ok := mf[name]
	if !ok {
		return 0
	}
	return len(fam.GetMetric())
}

func approxEqual(a, b float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(a - b)
	if b == 0 {
		return diff < 1e-15
	}
	return diff/math.Abs(b) < 1e-9
}
