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

	reg        *prometheus.Registry
	everInSync bool

	// PHC
	inSyncGauge         prometheus.Gauge
	offsetHistogram     prometheus.Histogram
	offsetGauge         prometheus.Gauge
	frequencyGauge      prometheus.Gauge
	offsetAbsSumCounter prometheus.Counter
	offsetSumSqCounter  prometheus.Counter
	samplesCounter      *prometheus.CounterVec

	// Satellites
	azimuthGauge       *prometheus.GaugeVec
	elevationGauge     *prometheus.GaugeVec
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

	azimuthGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "satpulse_satellite_azimuth_degrees",
		Help: "Azimuth of satellite in degrees",
	}, []string{"gnss", "sv"})

	elevationGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "satpulse_satellite_elevation_degrees",
		Help: "Elevation of satellite in degrees",
	}, []string{"gnss", "sv"})

	satelliteUsedGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "satpulse_satellite_used",
		Help: "Flags that a satellite is used in the navigation solution (1 = used)",
	}, []string{"gnss", "sv"})

	signalLevelGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "satpulse_satellite_signal_level_dbhz",
		Help: "Signal level of satellite in dB-Hz",
	}, []string{"gnss", "sv", "signal"})

	// Register metrics
	reg.MustRegister(inSyncGauge)
	reg.MustRegister(offsetHistogram)
	reg.MustRegister(offsetGauge)
	reg.MustRegister(frequencyGauge)
	reg.MustRegister(offsetAbsSumCounter)
	reg.MustRegister(offsetSumSqCounter)
	reg.MustRegister(samplesCounter)
	reg.MustRegister(azimuthGauge)
	reg.MustRegister(elevationGauge)
	reg.MustRegister(satelliteUsedGauge)
	reg.MustRegister(signalLevelGauge)

	return &PrometheusObserver{
		reg:                 reg,
		inSyncGauge:         inSyncGauge,
		offsetHistogram:     offsetHistogram,
		offsetGauge:         offsetGauge,
		frequencyGauge:      frequencyGauge,
		offsetAbsSumCounter: offsetAbsSumCounter,
		offsetSumSqCounter:  offsetSumSqCounter,
		samplesCounter:      samplesCounter,
		azimuthGauge:        azimuthGauge,
		elevationGauge:      elevationGauge,
		satelliteUsedGauge:  satelliteUsedGauge,
		signalLevelGauge:    signalLevelGauge,
	}
}

// makeBuckets generates histogram buckets based on the clock accuracy
// If a reasonable clockAccuracy is provided, it will be included in the buckets.
func makeBuckets(clockAccuracyNanos float64) []float64 {
	// These are buckets in seconds
	// We are trying to keep to under 50 buckets for Prometheus performance.
	var buckets = []float64{
		1e-9, 2e-9, 5e-9,
		10e-9, 15e-9, 20e-9, 25e-9, 30e-9, 40e-9, 50e-9, 75e-9,
		100e-9, 150e-9, 200e-9, 250e-9, 300e-9, 400e-9, 500e-9, 750e-9,
		1000e-9, 1500e-9, 2000e-9, 2500e-9, 3000e-9, 4000e-9, 5000e-9, 7500e-9,
		10e-6, 15e-6, 20e-6, 25e-6, 30e-6, 40e-6, 50e-6, 75e-6,
		100e-6, 150e-6, 200e-6, 250e-6, 300e-6, 400e-6, 500e-6, 750e-6,
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
	}
}

func (p *PrometheusObserver) Satellites(msg *gpsprot.SatellitesMsg, _ time.Time) {
	for _, sv := range msg.SVs {
		labels := []string{sv.ID.GNSS.String(), fmt.Sprintf("%02d", sv.ID.PRN)}
		p.azimuthGauge.WithLabelValues(labels...).Set(float64(sv.Azimuth))
		p.elevationGauge.WithLabelValues(labels...).Set(float64(sv.Elevation))
		if msg.UsedValid {
			p.satelliteUsedGauge.WithLabelValues(labels...).Set(1)
		}
		for _, sig := range sv.Signals {
			if sig.CN0 == 0 {
				continue
			}
			sigLabels := append(labels, string(sig.ID))
			p.signalLevelGauge.WithLabelValues(sigLabels...).Set(float64(sig.CN0))
		}
	}
}
