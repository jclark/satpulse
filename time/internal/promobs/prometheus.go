package promobs

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/time/internal/obs"
	"github.com/jclark/satpulse/time/internal/phcsync"
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

	// Position metrics (lazily registered)
	positionDegrees *prometheus.GaugeVec
	heightMeters    *prometheus.GaugeVec

	// Solution quality metrics (lazily registered)
	gnssFix            *prometheus.GaugeVec
	seenFixLevels      map[string]struct{}
	gnssSolution       *prometheus.GaugeVec
	seenSolutionDims   map[string]struct{}
	numSatellites      *prometheus.GaugeVec
	dop                *prometheus.GaugeVec
	positionAccuracy   *prometheus.GaugeVec
	speedAccuracy      *prometheus.GaugeVec
	courseAccuracy      *prometheus.GaugeVec
	gnssCorrection     *prometheus.GaugeVec
	seenCorrExpanded   map[string]struct{}
	gnssCorrectionLeaf *prometheus.GaugeVec
	seenCorrLeaf       map[string]struct{}
	correctionAge      *prometheus.GaugeVec
	correctionBaseID   *prometheus.GaugeVec
	navAuxSource       *prometheus.GaugeVec
	seenAuxSrc         map[string]struct{}

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

// Sample implements phcsync.Sampler interface
func (p *PrometheusObserver) Sample(data phcsync.Sample) {
	// Always update frequency gauge (convert ppb to dimensionless)
	p.frequencyGauge.Set(data.Freq / 1e9)

	if data.Mode.InSync() {
		p.inSyncGauge.Set(1)
		if !p.everInSync {
			p.everInSync = true
		}
	} else {
		p.inSyncGauge.Set(0)
		if p.everInSync {
			p.samplesCounter.WithLabelValues("out_of_sync").Inc()
		} else {
			p.samplesCounter.WithLabelValues("pre_sync").Inc()
		}
		return // No need to process further if out of sync
	}

	switch data.Kind {
	case phcsync.SampleMissing:
		p.samplesCounter.WithLabelValues("missing").Inc()
	case phcsync.SampleOutlier:
		p.samplesCounter.WithLabelValues("outlier").Inc()
		// Update offset gauge for outliers too (convert to seconds)
		p.offsetGauge.Set(data.Offset.Seconds())
	case phcsync.SampleOK:
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

// NavEpochPV sets position and solution quality gauges.
// Gauges are lazily registered on first data, so they never appear if the
// receiver does not report that field.
func (p *PrometheusObserver) NavEpochPV(msg *gpsprot.NavEpochMsg, pv *gpsprot.PVMsgBundle, _ time.Time) {
	p.updatePosition(pv)
	p.updateSolutionQuality(msg)
}

func (p *PrometheusObserver) updatePosition(pv *gpsprot.PVMsgBundle) {
	if !pv.PosGeo.IsSet() {
		return
	}
	geo := pv.PosGeo.Get()
	if p.positionDegrees == nil {
		p.positionDegrees = registerLabelledGaugeVec(p.reg, "satpulse_position_degrees", "Geodetic position in degrees", "coord")
	}
	p.positionDegrees.WithLabelValues("latitude").Set(geo.LatLon[0].Degrees())
	p.positionDegrees.WithLabelValues("longitude").Set(geo.LatLon[1].Degrees())
	setOrDeleteOptLabel(p, &p.heightMeters, "satpulse_height_meters", "Height in meters", "type",
		"ellipsoid", geo.Height.IsSet(), func() float64 { return geo.Height.Get().Meters() })
	setOrDeleteOptLabel(p, &p.heightMeters, "satpulse_height_meters", "Height in meters", "type",
		"msl", geo.HeightMSL.IsSet(), func() float64 { return geo.HeightMSL.Get().Meters() })
}

func (p *PrometheusObserver) updateSolutionQuality(msg *gpsprot.NavEpochMsg) {
	// Fix level
	if msg.FixLevel != 0 {
		label := camelToSnake(msg.FixLevel.String())
		p.seenFixLevels = setInfoGauge(p, &p.gnssFix, "satpulse_gnss_fix", "GNSS fix level", "level",
			label, p.seenFixLevels)
	} else {
		clearInfoGauge(p.gnssFix, p.seenFixLevels)
	}
	// Solution dimensionality
	if msg.SolutionDim != 0 {
		label := camelToSnake(msg.SolutionDim.String())
		p.seenSolutionDims = setInfoGauge(p, &p.gnssSolution, "satpulse_gnss_solution", "GNSS solution dimensionality", "dim",
			label, p.seenSolutionDims)
	} else {
		clearInfoGauge(p.gnssSolution, p.seenSolutionDims)
	}
	// Satellite counts
	setOrDeleteOpt(p, &p.numSatellites, "satpulse_num_satellites", "Number of satellites", "status",
		"used", msg.NumSVUsed.IsSet(), func() float64 { return float64(msg.NumSVUsed.Get()) })
	setOrDeleteOpt(p, &p.numSatellites, "satpulse_num_satellites", "Number of satellites", "status",
		"tracked", msg.NumSVTracked.IsSet(), func() float64 { return float64(msg.NumSVTracked.Get()) })
	setOrDeleteOpt(p, &p.numSatellites, "satpulse_num_satellites", "Number of satellites", "status",
		"in_view", msg.NumSVInView.IsSet(), func() float64 { return float64(msg.NumSVInView.Get()) })
	// DOP
	setOrDeleteOpt(p, &p.dop, "satpulse_dop", "Dilution of precision", "type",
		"geometric", msg.DOP.Geom.IsSet(), func() float64 { return msg.DOP.Geom.Get() })
	setOrDeleteOpt(p, &p.dop, "satpulse_dop", "Dilution of precision", "type",
		"position", msg.DOP.Pos.IsSet(), func() float64 { return msg.DOP.Pos.Get() })
	setOrDeleteOpt(p, &p.dop, "satpulse_dop", "Dilution of precision", "type",
		"horizontal", msg.DOP.Hor.IsSet(), func() float64 { return msg.DOP.Hor.Get() })
	setOrDeleteOpt(p, &p.dop, "satpulse_dop", "Dilution of precision", "type",
		"vertical", msg.DOP.Vert.IsSet(), func() float64 { return msg.DOP.Vert.Get() })
	setOrDeleteOpt(p, &p.dop, "satpulse_dop", "Dilution of precision", "type",
		"time", msg.DOP.Time.IsSet(), func() float64 { return msg.DOP.Time.Get() })
	// Position accuracy
	setOrDeleteOpt(p, &p.positionAccuracy, "satpulse_position_accuracy_meters", "Position accuracy in meters", "type",
		"horizontal", msg.Acc.Hor.IsSet(), func() float64 { return msg.Acc.Hor.Get().Meters() })
	setOrDeleteOpt(p, &p.positionAccuracy, "satpulse_position_accuracy_meters", "Position accuracy in meters", "type",
		"vertical", msg.Acc.Vert.IsSet(), func() float64 { return msg.Acc.Vert.Get().Meters() })
	setOrDeleteOpt(p, &p.positionAccuracy, "satpulse_position_accuracy_meters", "Position accuracy in meters", "type",
		"3d", msg.Acc.Pos.IsSet(), func() float64 { return msg.Acc.Pos.Get().Meters() })
	// Speed accuracy
	setOrDeleteOpt(p, &p.speedAccuracy, "satpulse_speed_accuracy_meters_per_second", "Speed accuracy in m/s", "type",
		"3d", msg.Acc.Speed.IsSet(), func() float64 { return msg.Acc.Speed.Get().MetersPerSecond() })
	setOrDeleteOpt(p, &p.speedAccuracy, "satpulse_speed_accuracy_meters_per_second", "Speed accuracy in m/s", "type",
		"ground", msg.Acc.GroundSpeed.IsSet(), func() float64 { return msg.Acc.GroundSpeed.Get().MetersPerSecond() })
	// Course accuracy
	setOrDeleteOptNoLabel(p, &p.courseAccuracy, "satpulse_course_accuracy_degrees", "Course accuracy in degrees",
		msg.Acc.Course.IsSet(), func() float64 { return msg.Acc.Course.Get().Degrees() })
	// Corrections
	if msg.Correction != 0 {
		p.seenCorrExpanded = setBitmaskGauge(p, &p.gnssCorrection, "satpulse_gnss_correction", "GNSS correction (all implied bits)", "kind",
			corrSnakeNames(msg.Correction.Expand()), p.seenCorrExpanded)
		p.seenCorrLeaf = setBitmaskGauge(p, &p.gnssCorrectionLeaf, "satpulse_gnss_correction_leaf", "GNSS correction (leaf only)", "kind",
			corrSnakeNames(msg.Correction.Leaves()), p.seenCorrLeaf)
	} else {
		clearBitmaskGauge(p.gnssCorrection, p.seenCorrExpanded)
		clearBitmaskGauge(p.gnssCorrectionLeaf, p.seenCorrLeaf)
	}
	// Correction metadata
	setOrDeleteOptNoLabel(p, &p.correctionAge, "satpulse_correction_age_seconds", "Age of differential corrections in seconds",
		msg.DiffAge.IsSet(), func() float64 { return msg.DiffAge.Get().Seconds() })
	setOrDeleteOptNoLabel(p, &p.correctionBaseID, "satpulse_correction_base_id", "RTCM reference base station ID",
		msg.RTCMRefBaseID.IsSet(), func() float64 { return float64(msg.RTCMRefBaseID.Get()) })
	// Auxiliary sources
	if msg.AuxSrc != 0 {
		names := make([]string, 0, 2)
		for _, s := range msg.AuxSrc.Items() {
			names = append(names, strings.ToLower(s))
		}
		p.seenAuxSrc = setBitmaskGauge(p, &p.navAuxSource, "satpulse_nav_aux_source", "Auxiliary navigation source", "kind",
			names, p.seenAuxSrc)
	} else {
		clearBitmaskGauge(p.navAuxSource, p.seenAuxSrc)
	}
}

// setOrDeleteOpt handles a labelled opt.Val-backed metric: registers the GaugeVec
// on first set, sets value when present, deletes the label value when absent.
func setOrDeleteOpt(p *PrometheusObserver, gv **prometheus.GaugeVec, name, help, labelName, labelVal string, isSet bool, val func() float64) {
	if isSet {
		if *gv == nil {
			*gv = registerLabelledGaugeVec(p.reg, name, help, labelName)
		}
		(*gv).WithLabelValues(labelVal).Set(val())
	} else if *gv != nil {
		(*gv).DeleteLabelValues(labelVal)
	}
}

// setOrDeleteOptLabel is like setOrDeleteOpt but reuses the same name/help for
// a GaugeVec that may already be registered (e.g. height_meters with two label values).
func setOrDeleteOptLabel(p *PrometheusObserver, gv **prometheus.GaugeVec, name, help, labelName, labelVal string, isSet bool, val func() float64) {
	if isSet {
		if *gv == nil {
			*gv = registerLabelledGaugeVec(p.reg, name, help, labelName)
		}
		(*gv).WithLabelValues(labelVal).Set(val())
	} else if *gv != nil {
		(*gv).DeleteLabelValues(labelVal)
	}
}

// setOrDeleteOptNoLabel handles a label-free opt.Val-backed metric.
func setOrDeleteOptNoLabel(p *PrometheusObserver, gv **prometheus.GaugeVec, name, help string, isSet bool, val func() float64) {
	if isSet {
		if *gv == nil {
			*gv = registerGaugeVec(p.reg, name, help)
		}
		(*gv).WithLabelValues().Set(val())
	} else if *gv != nil {
		(*gv).DeleteLabelValues()
	}
}

// setInfoGauge handles bitmask/info-style metrics where the active label value
// is set to 1 and all previously seen values are set to 0.
func setInfoGauge(p *PrometheusObserver, gv **prometheus.GaugeVec, name, help, labelName, activeVal string, seen map[string]struct{}) map[string]struct{} {
	if *gv == nil {
		*gv = registerLabelledGaugeVec(p.reg, name, help, labelName)
		seen = make(map[string]struct{})
	}
	for prev := range seen {
		if prev != activeVal {
			(*gv).WithLabelValues(prev).Set(0)
		}
	}
	(*gv).WithLabelValues(activeVal).Set(1)
	seen[activeVal] = struct{}{}
	return seen
}

// clearInfoGauge sets all previously seen label values to 0.
// Used when the field is absent/unset for the current epoch.
func clearInfoGauge(gv *prometheus.GaugeVec, seen map[string]struct{}) {
	if gv == nil {
		return
	}
	for prev := range seen {
		gv.WithLabelValues(prev).Set(0)
	}
}

// setBitmaskGauge handles bitmask metrics where multiple label values can be
// active (1) simultaneously, and previously seen but now inactive values are 0.
func setBitmaskGauge(p *PrometheusObserver, gv **prometheus.GaugeVec, name, help, labelName string, activeNames []string, seen map[string]struct{}) map[string]struct{} {
	if *gv == nil {
		*gv = registerLabelledGaugeVec(p.reg, name, help, labelName)
		seen = make(map[string]struct{})
	}
	active := make(map[string]struct{}, len(activeNames))
	for _, n := range activeNames {
		active[n] = struct{}{}
	}
	for prev := range seen {
		if _, ok := active[prev]; !ok {
			(*gv).WithLabelValues(prev).Set(0)
		}
	}
	for _, n := range activeNames {
		(*gv).WithLabelValues(n).Set(1)
		seen[n] = struct{}{}
	}
	return seen
}

// clearBitmaskGauge sets all previously seen label values to 0.
func clearBitmaskGauge(gv *prometheus.GaugeVec, seen map[string]struct{}) {
	if gv == nil {
		return
	}
	for prev := range seen {
		gv.WithLabelValues(prev).Set(0)
	}
}

// corrSnakeNames converts CorrKind names to snake_case.
func corrSnakeNames(c gpsprot.CorrKind) []string {
	names := c.Names()
	for i, n := range names {
		names[i] = camelToSnake(n)
	}
	return names
}

func registerGaugeVec(reg *prometheus.Registry, name, help string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, nil)
	reg.MustRegister(g)
	return g
}

func registerLabelledGaugeVec(reg *prometheus.Registry, name, help, label string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, []string{label})
	reg.MustRegister(g)
	return g
}

// camelToSnake converts camelCase/PascalCase to snake_case.
// Uppercase runs stay together as one word (e.g. "PPPConverging" -> "ppp_converging").
// Hyphens are word separators.
func camelToSnake(s string) string {
	words := splitCamelWords(s)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "_")
}

// splitCamelWords splits a camelCase/PascalCase string into words.
// Hyphens are treated as word boundaries and discarded.
func splitCamelWords(s string) []string {
	var words []string
	start := 0
	upperRunLen := 0
	for i, c := range s {
		if c == '-' {
			if i > start {
				words = append(words, s[start:i])
			}
			start = i + 1
			upperRunLen = 0
			continue
		}
		upper := 'A' <= c && c <= 'Z'
		if upper {
			if upperRunLen == 0 && i > start && s[i-1] >= 'a' && s[i-1] <= 'z' {
				// Uppercase after lowercase: new word.
				words = append(words, s[start:i])
				start = i
			}
			upperRunLen++
		} else {
			if upperRunLen > 1 {
				// Lowercase after uppercase run: the last uppercase char
				// belongs to this new word, not the previous run.
				words = append(words, s[start:i-1])
				start = i - 1
			}
			upperRunLen = 0
		}
	}
	if start < len(s) {
		words = append(words, s[start:])
	}
	return words
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
