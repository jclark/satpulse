package promobs

import (
	"math"
	"net/http"
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/obs"
)

// PrometheusObserver implements obs.Observer for Prometheus metrics
type PrometheusObserver struct {
	obs.DefaultObserver

	reg        *prometheus.Registry
	everInSync bool

	inSyncGauge           prometheus.Gauge
	offsetHistogram       prometheus.Histogram
	offsetGauge           prometheus.Gauge
	frequencyGauge        prometheus.Gauge
	offsetAbsSumCounter   prometheus.Counter
	offsetSumSqCounter    prometheus.Counter
	missingCounter        prometheus.Counter
	outlierCounter        prometheus.Counter
	outOfSyncCounter      prometheus.Counter
	goodCounter           prometheus.Counter
	untilFirstSyncCounter prometheus.Counter
}

// New creates a new PrometheusObserver with basic metrics
func New(clockAccuracyNanos int) *PrometheusObserver {
	reg := prometheus.NewRegistry()

	// Sync state gauge
	inSyncGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "satpulse_in_sync",
		Help: "Synchronization status (1 = in sync, 0 = out of sync)",
	})

	// Clock offset histogram
	offsetHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "satpulse_clock_abs_offset_nanoseconds",
		Help:    "Histogram of absolute offset between PHC and GPS time in nanoseconds",
		Buckets: makeBuckets(float64(clockAccuracyNanos)),
	})

	// Clock offset gauge (signed)
	offsetGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "satpulse_clock_offset_nanoseconds",
		Help: "Current signed offset between PHC and GPS time in nanoseconds",
	})

	// Clock frequency gauge
	frequencyGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "satpulse_clock_frequency_ppb",
		Help: "Current frequency adjustment in parts per billion",
	})

	// Statistical counters
	offsetAbsSumCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_offset_abs_sum_nanoseconds_total",
		Help: "Sum of absolute values of offsets from good samples (for mean calculation)",
	})

	offsetSumSqCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_offset_sum_squares_nanoseconds_total",
		Help: "Sum of squares of offsets from good samples (for stddev calculation)",
	})

	// Sample tracking counters
	missingCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_samples_missing_total",
		Help: "Total number of missing samples (time pulse not received)",
	})

	outlierCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_samples_outlier_total",
		Help: "Total number of outlier samples (not used by servo)",
	})

	outOfSyncCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_samples_out_of_sync_total",
		Help: "Total number of samples when out of sync",
	})

	goodCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_samples_good_total",
		Help: "Total number of good samples (in sync and not outlier)",
	})

	samplesUntilFirstSyncCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "satpulse_samples_until_first_sync_total",
		Help: "Total number of samples processed before achieving first sync",
	})

	// Register metrics
	reg.MustRegister(inSyncGauge)
	reg.MustRegister(offsetHistogram)
	reg.MustRegister(offsetGauge)
	reg.MustRegister(frequencyGauge)
	reg.MustRegister(offsetAbsSumCounter)
	reg.MustRegister(offsetSumSqCounter)
	reg.MustRegister(missingCounter)
	reg.MustRegister(outlierCounter)
	reg.MustRegister(outOfSyncCounter)
	reg.MustRegister(goodCounter)
	reg.MustRegister(samplesUntilFirstSyncCounter)

	return &PrometheusObserver{
		reg:                   reg,
		inSyncGauge:           inSyncGauge,
		offsetHistogram:       offsetHistogram,
		offsetGauge:           offsetGauge,
		frequencyGauge:        frequencyGauge,
		offsetAbsSumCounter:   offsetAbsSumCounter,
		offsetSumSqCounter:    offsetSumSqCounter,
		missingCounter:        missingCounter,
		outlierCounter:        outlierCounter,
		outOfSyncCounter:      outOfSyncCounter,
		goodCounter:           goodCounter,
		untilFirstSyncCounter: samplesUntilFirstSyncCounter,
	}
}

// makeBuckets generates histogram buckets based on the clock accuracy
// If a reasonable clockAccuracy is provided, it will be included in the buckets.
func makeBuckets(clockAccuracy float64) []float64 {
	// These are buckets in nanoseconds
	// We are trying to keep to under 50 buckets for Prometheus performance.
	var buckets = []float64{
		1, 2, 5,
		10, 15, 20, 25, 30, 40, 50, 75,
		10e1, 15e1, 20e1, 25e1, 30e1, 40e1, 50e1, 75e1,
		10e2, 15e2, 20e2, 25e2, 30e2, 40e2, 50e2, 75e2,
		10e3, 15e3, 20e3, 25e3, 30e3, 40e3, 50e3, 75e3,
		10e4, 15e4, 20e4, 25e4, 30e4, 40e4, 50e4, 75e4,
		1e6, // last bucket is a millisecond; for PTP that is an eternity
	}
	if clockAccuracy <= 0 {
		return buckets
	}
	i, ok := slices.BinarySearch(buckets, clockAccuracy)
	if ok {
		return buckets
	}
	return slices.Insert(buckets, i, clockAccuracy)
}

// Handler returns the HTTP handler for /metrics endpoint
func (p *PrometheusObserver) Handler() http.Handler {
	return promhttp.HandlerFor(p.reg, promhttp.HandlerOpts{})
}

// Sample implements mon.Sampler interface
func (p *PrometheusObserver) Sample(data mon.SampleData) {
	// Always update frequency gauge
	p.frequencyGauge.Set(data.Freq)

	switch data.SyncState {
	case mon.InSync:
		p.inSyncGauge.Set(1)
		if !p.everInSync {
			p.everInSync = true
		}
	case mon.NoSync:
		p.inSyncGauge.Set(0)
		if p.everInSync {
			p.outOfSyncCounter.Inc()
		} else {
			p.untilFirstSyncCounter.Inc()
		}
		return // No need to process further if out of sync
	}

	switch data.Kind {
	case mon.SampleMissing:
		p.missingCounter.Inc()
	case mon.SampleOutlier:
		p.outlierCounter.Inc()
		// Update offset gauge for outliers too
		p.offsetGauge.Set(float64(data.Offset.Nanoseconds()))
	case mon.SampleOK:
		p.goodCounter.Inc()
		offsetNs := float64(data.Offset.Nanoseconds())
		p.offsetGauge.Set(offsetNs)
		p.offsetHistogram.Observe(math.Abs(offsetNs))
		p.offsetAbsSumCounter.Add(math.Abs(offsetNs))
		p.offsetSumSqCounter.Add(offsetNs * offsetNs)
	}
}
