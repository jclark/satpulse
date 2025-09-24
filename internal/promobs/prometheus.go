package promobs

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
)

// PrometheusObserver implements obs.Observer for Prometheus metrics
type PrometheusObserver struct {
	obs.DefaultObserver

	reg              *prometheus.Registry
	everInSync       bool
	activeSatellites map[gpsprot.SVID]map[gpsprot.SignalID]struct{}

	// PHC metrics
	inSyncGauge           prometheus.Gauge
	offsetHistogram       prometheus.Histogram
	offsetGauge           prometheus.Gauge
	frequencyGauge        prometheus.Gauge
	offsetAbsSumCounter   prometheus.Counter
	offsetSumSqCounter    prometheus.Counter
	samplesCounter        *prometheus.CounterVec
	freqDeltaCountCounter prometheus.Counter
	freqDeltaSumGauge     prometheus.Counter
	freqDeltaSumSqCounter prometheus.Counter

	// Satellites metrics
	lookAngleGauge     *prometheus.GaugeVec
	satelliteUsedGauge *prometheus.GaugeVec
	signalLevelGauge   *prometheus.GaugeVec
}

// New creates a new PrometheusObserver with basic metrics
func New(clockAccuracyNanos int) *PrometheusObserver {
	reg := prometheus.NewRegistry()

	// Sync state gauge
	inSyncGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "satpulse_phc_sync_status",
		Help: "PHC synchronization status (0 = out of sync, 1 = in sync)",
	})
	// It takes a while for samples to start coming through: during this period we should be observed as out of sync.
	inSyncGauge.Set(0)

	// Clock offset histogram
	offsetHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "satpulse_phc_offset_abs_seconds",
		Help:    "Absolute offset between PHC and GPS time in seconds",
		Buckets: makeBuckets(float64(clockAccuracyNanos)),
	})

	// Clock offset gauge (signed)
	offsetGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "satpulse_phc_offset_seconds",
		Help: "Current signed offset between PHC and GPS time in seconds",
	})

	// Clock frequency gauge
	frequencyGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "satpulse_phc_frequency_adjustment",
		Help: "Current PHC frequency adjustment (dimensionless)",
	})

	// Statistical counters
	offsetAbsSumCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_phc_offset_abs_sum_seconds_total",
		Help: "Sum of absolute values of offsets from OK samples (for mean calculation)",
	})

	offsetSumSqCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_phc_offset_sum_squares_seconds_total",
		Help: "Sum of squares of offsets from OK samples (for stddev calculation)",
	})

	// Sample tracking counter with status labels
	samplesCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "satpulse_phc_samples_total",
		Help: "Total number of PHC samples by status",
	}, []string{"status"})

	// Frequency delta statistics
	freqDeltaCountCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_phc_frequency_delta_count_total",
		Help: "Total number of frequency delta samples from OK samples",
	})
	freqDeltaSumGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "satpulse_phc_frequency_delta_sum_ppb_total",
		Help: "Sum of frequency delta values from OK samples (for mean calculation)",
	})
	freqDeltaSumSqCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_phc_frequency_delta_sum_squares_ppb_total",
		Help: "Sum of squares of frequency delta values from OK samples (for stddev calculation)",
	})

	lookAngleGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "satpulse_satellite_look_angle_degrees",
		Help: "Look angle (direction) of satellite in degrees",
	}, []string{"gnss", "sv", "angle"})

	satelliteUsedGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "satpulse_satellite_used",
		Help: "Flags that a satellite is used in the navigation solution (1 = used, 0 = not used)",
	}, []string{"gnss", "sv"})

	signalLevelGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "satpulse_satellite_signal_level_dbhz",
		Help: "Signal level of satellite in dB-Hz",
	}, []string{"gnss", "sv", "signal", "used"})

	// Register metrics
	reg.MustRegister(inSyncGauge)
	reg.MustRegister(offsetHistogram)
	reg.MustRegister(offsetGauge)
	reg.MustRegister(frequencyGauge)
	reg.MustRegister(offsetAbsSumCounter)
	reg.MustRegister(offsetSumSqCounter)
	reg.MustRegister(samplesCounter)
	reg.MustRegister(freqDeltaCountCounter)
	reg.MustRegister(freqDeltaSumGauge)
	reg.MustRegister(freqDeltaSumSqCounter)
	reg.MustRegister(lookAngleGauge)
	reg.MustRegister(satelliteUsedGauge)
	reg.MustRegister(signalLevelGauge)

	return &PrometheusObserver{
		reg:                   reg,
		activeSatellites:      make(map[gpsprot.SVID]map[gpsprot.SignalID]struct{}),
		inSyncGauge:           inSyncGauge,
		offsetHistogram:       offsetHistogram,
		offsetGauge:           offsetGauge,
		frequencyGauge:        frequencyGauge,
		offsetAbsSumCounter:   offsetAbsSumCounter,
		offsetSumSqCounter:    offsetSumSqCounter,
		samplesCounter:        samplesCounter,
		freqDeltaCountCounter: freqDeltaCountCounter,
		freqDeltaSumGauge:     freqDeltaSumGauge,
		freqDeltaSumSqCounter: freqDeltaSumSqCounter,
		lookAngleGauge:        lookAngleGauge,
		satelliteUsedGauge:    satelliteUsedGauge,
		signalLevelGauge:      signalLevelGauge,
	}
}

// makeBuckets generates histogram buckets based on the clock accuracy
// If a reasonable clockAccuracy is provided, it will be included in the buckets.
func makeBuckets(clockAccuracyNanos float64) []float64 {
	// These are buckets in seconds
	// We are trying to keep to under 50 buckets for Prometheus performance.
	var buckets = []float64{
		1e-9, 2e-9, 5e-9,
		// keep it quite dense up 100 nanoseconds, since these are values we are likely to see
		10e-9, 15e-9, 20e-9, 25e-9, 30e-9, 35e-9, 40e-9, 45e-9, 50e-9,
		55e-9, 60e-9, 65e-9, 70e-9, 75e-9, 80e-9, 85e-9, 90e-9, 95e-9,
		100e-9, 110e-9, 120e-9, 130e-9, 140e-9, 150e-9, 170e-9,
		200e-9, 250e-9, 300e-9, 350e-9, 400e-9, 450e-9, 500e-9, 600e-9, 750e-9,
		// these are the PTP clock accuracy buckets; not likely we will ever see these
		1000e-9, 2500e-9,
		10e-6, 25e-6,
		100e-6, 250e-6,
		1e-3, // last bucket is a millisecond; for PTP that is an eternity
	}
	if clockAccuracyNanos <= 0 {
		return buckets
	}
	clockAccuracySeconds := clockAccuracyNanos / 1e9
	i, ok := slices.BinarySearch(buckets, clockAccuracySeconds)
	if ok {
		return buckets
	}
	return slices.Insert(buckets, i, clockAccuracySeconds)
}

// Handler returns the HTTP handler for /metrics endpoint
func (p *PrometheusObserver) Handler() http.Handler {
	return promhttp.HandlerFor(p.reg, promhttp.HandlerOpts{})
}

// Sample implements mon.Sampler interface
func (p *PrometheusObserver) Sample(data mon.SampleData) {
	// Always update frequency gauge (convert ppb to dimensionless)
	p.frequencyGauge.Set(data.Freq / 1e9)

	switch data.SyncState {
	case mon.InSync:
		p.inSyncGauge.Set(1)
		if !p.everInSync {
			p.everInSync = true
		}
	case mon.NoSync:
		p.inSyncGauge.Set(0)
		if p.everInSync {
			p.samplesCounter.WithLabelValues("out_of_sync").Inc()
		} else {
			p.samplesCounter.WithLabelValues("pre_sync").Inc()
		}
		return // No need to process further if out of sync
	}

	switch data.Kind {
	case mon.SampleMissing:
		p.samplesCounter.WithLabelValues("missing").Inc()
	case mon.SampleOutlier:
		p.samplesCounter.WithLabelValues("outlier").Inc()
		// Update offset gauge for outliers too (convert to seconds)
		p.offsetGauge.Set(data.Offset.Seconds())
	case mon.SampleOK:
		p.samplesCounter.WithLabelValues("ok").Inc()
		offsetSeconds := data.Offset.Seconds()
		p.offsetGauge.Set(offsetSeconds)
		p.offsetHistogram.Observe(math.Abs(offsetSeconds))
		p.offsetAbsSumCounter.Add(math.Abs(offsetSeconds))
		p.offsetSumSqCounter.Add(offsetSeconds * offsetSeconds)
		p.freqDeltaCountCounter.Inc()
		p.freqDeltaSumGauge.Add(data.FreqDelta)
		p.freqDeltaSumSqCounter.Add(data.FreqDelta * data.FreqDelta)
	}
}

func (p *PrometheusObserver) Satellites(msg *gpsprot.SatellitesMsg, _ time.Time) {
	// This is more complicated that one might expect.
	// Prometheus gauges retain their last Set() value until explicitly deleted or updated.
	// Since we only call Set() for currently active satellites/signals, we must actively
	// delete metrics for satellites or signals that are no longer present.
	// Otherwise, stale metrics would continue to be exported on each scrape indefinitely;
	// this would lead to Grafana showing the higher water mark of satellites/signals.

	prevSats := p.activeSatellites
	curSats := make(map[gpsprot.SVID]map[gpsprot.SignalID]struct{})

	for _, s := range msg.SVs {

		gnss := s.ID.GNSS.String()
		sv := fmt.Sprintf("%02d", s.ID.Num)
		if s.ID.Num == gpsprot.GLOUnknown && s.ID.GNSS == gpsprot.GLO {
			// GLONASS satellites where orbital slot is not yet known
			sv = "?"
		}

		curSigs := make(map[gpsprot.SignalID]struct{})

		for _, sig := range s.Signals {
			if sig.CN0 == 0 {
				continue
			}
			curSigs[sig.ID] = struct{}{}
			p.signalLevelGauge.WithLabelValues(gnss, sv, signalIDString(sig.ID), usedLabel(msg.UsedValidity, s, sig)).Set(float64(sig.CN0))
		}

		if len(curSigs) == 0 {
			continue
		}

		// Clean up stale signals for this satellite
		if prevSigs, exists := prevSats[s.ID]; exists {
			for sigID := range prevSigs {
				if _, stillExists := curSigs[sigID]; !stillExists {
					p.signalLevelGauge.DeletePartialMatch(prometheus.Labels{"gnss": gnss, "sv": sv, "signal": signalIDString(sigID)})
				}
			}
		}

		curSats[s.ID] = curSigs
		// Always set used state (0 or 1) for every active satellite.
		usedValue := float64(0)
		if msg.UsedValidity >= gpsprot.SatelliteUsedSV && s.Used {
			usedValue = 1
		}
		p.satelliteUsedGauge.WithLabelValues(gnss, sv).Set(usedValue)
		if s.LookAngles == nil {
			continue
		}
		lookAngles := s.LookAngles
		p.lookAngleGauge.WithLabelValues(gnss, sv, "azimuth").Set(float64(lookAngles.Azimuth))
		p.lookAngleGauge.WithLabelValues(gnss, sv, "elevation").Set(float64(lookAngles.Elevation))
	}

	// Clean up satellites that are no longer present
	for svid := range prevSats {
		if _, exists := curSats[svid]; !exists {
			gnss := svid.GNSS.String()
			sv := fmt.Sprintf("%02d", svid.Num)
			p.signalLevelGauge.DeletePartialMatch(prometheus.Labels{"gnss": gnss, "sv": sv})
			p.lookAngleGauge.DeletePartialMatch(prometheus.Labels{"gnss": gnss, "sv": sv})
			p.satelliteUsedGauge.DeleteLabelValues(gnss, sv)
		}
	}

	p.activeSatellites = curSats
}

func usedLabel(usedValidity gpsprot.SatelliteUsedValidity, sv gpsprot.SVInfo, sig gpsprot.SignalInfo) string {
	var used bool
	switch usedValidity {
	case gpsprot.SatelliteUsedSignal:
		used = sig.Used
	case gpsprot.SatelliteUsedSV:
		if len(sv.Signals) == 1 {
			// if the SV only has one signal,
			// we can infer the signal's usage from the SV's usage
			used = sv.Used
			break
		}
		fallthrough // yeah, I used this Go feature once!
	default:
		return "unknown"
	}
	if used {
		return "true"
	}
	return "false"
}

func signalIDString(id gpsprot.SignalID) string {
	if id == gpsprot.SigIDInvalid {
		return "L?"
	}
	return string(id)
}
