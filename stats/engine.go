package stats

import (
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	DriftEWMAAlpha       float64 `yaml:"drift_ewma_alpha"`
	ProcessEWMAAlpha     float64 `yaml:"process_ewma_alpha"`
	RemediationEWMAAlpha float64 `yaml:"remediation_ewma_alpha"`

	MinInterval      time.Duration `yaml:"min_interval"`
	SeriesTTL        time.Duration `yaml:"series_ttl"`
	CleanupInterval  time.Duration `yaml:"cleanup_interval"`
	MaxProcessSeries int           `yaml:"max_process_series"`

	Anomaly    AnomalyConfig    `yaml:"anomaly"`
	Burst      BurstConfig      `yaml:"burst"`
	Reputation ReputationConfig `yaml:"reputation"`
}

func (c Config) withDefaults() Config {
	out := c
	if out.DriftEWMAAlpha == 0 {
		out.DriftEWMAAlpha = 0.3
	}
	if out.ProcessEWMAAlpha == 0 {
		out.ProcessEWMAAlpha = 0.3
	}
	if out.RemediationEWMAAlpha == 0 {
		out.RemediationEWMAAlpha = 0.3
	}

	if out.MinInterval <= 0 {
		out.MinInterval = 10 * time.Millisecond
	}
	if out.SeriesTTL <= 0 {
		out.SeriesTTL = 30 * time.Minute
	}
	if out.CleanupInterval <= 0 {
		out.CleanupInterval = 1 * time.Minute
	}
	if out.MaxProcessSeries <= 0 {
		out.MaxProcessSeries = 2048
	}

	out.Anomaly = out.Anomaly.withDefaults()
	out.Burst = out.Burst.withDefaults()
	out.Reputation = out.Reputation.withDefaults()
	return out
}

type RateMetrics struct {
	Current     float64
	EWMA        float64
	Mean        float64
	Variance    float64
	StdDev      float64
	SampleCount int64
	Anomaly     AnomalyResult
	Burst       BurstSnapshot
}

type EventIntel struct {
	ProcessWrite            RateMetrics
	ParamDrift              RateMetrics
	Remediation             RateMetrics
	ProcessReputationBefore ReputationSnapshot
	ProcessReputationAfter  ReputationSnapshot

	BehavioralAnomaly         bool
	BehavioralAnomalySeverity Severity
	Burst                     BurstSnapshot
}

type Engine struct {
	cfg Config

	processWrites *rateSeriesManager
	paramDrifts   *rateSeriesManager
	remediations  *rateSeriesManager

	reputation *ReputationManager
}

func NewEngine(cfg Config) *Engine {
	cfg = cfg.withDefaults()

	processWrites := newRateSeriesManager(rateSeriesConfig{
		alpha:           cfg.ProcessEWMAAlpha,
		minInterval:     cfg.MinInterval,
		anomaly:         cfg.Anomaly,
		burst:           cfg.Burst,
		maxEntries:      cfg.MaxProcessSeries,
		entryTTL:        cfg.SeriesTTL,
		cleanupInterval: cfg.CleanupInterval,
	})

	// Parameter keys are bounded by policy size, but we still TTL them.
	paramDrifts := newRateSeriesManager(rateSeriesConfig{
		alpha:           cfg.DriftEWMAAlpha,
		minInterval:     cfg.MinInterval,
		anomaly:         cfg.Anomaly,
		burst:           cfg.Burst,
		maxEntries:      0,
		entryTTL:        cfg.SeriesTTL,
		cleanupInterval: cfg.CleanupInterval,
	})

	// Single key (global remediation frequency).
	remediations := newRateSeriesManager(rateSeriesConfig{
		alpha:           cfg.RemediationEWMAAlpha,
		minInterval:     cfg.MinInterval,
		anomaly:         cfg.Anomaly,
		burst:           cfg.Burst,
		maxEntries:      8,
		entryTTL:        cfg.SeriesTTL,
		cleanupInterval: cfg.CleanupInterval,
	})

	return &Engine{
		cfg:           cfg,
		processWrites: processWrites,
		paramDrifts:   paramDrifts,
		remediations:  remediations,
		reputation:    NewReputationManager(cfg.Reputation),
	}
}

func (e *Engine) ObserveProcessWrite(process string, now time.Time) RateMetrics {
	if process == "" {
		return RateMetrics{}
	}
	return e.processWrites.Observe(process, now)
}

func (e *Engine) ObserveParamDrift(param string, now time.Time) RateMetrics {
	if param == "" {
		return RateMetrics{}
	}
	return e.paramDrifts.Observe(param, now)
}

func (e *Engine) ObserveRemediation(now time.Time) RateMetrics {
	return e.remediations.Observe("__global__", now)
}

func (e *Engine) GetReputation(process string, now time.Time) ReputationSnapshot {
	return e.reputation.Get(process, now)
}

func (e *Engine) UpdateReputation(process string, now time.Time, upd ReputationUpdate) ReputationSnapshot {
	return e.reputation.Update(process, now, upd)
}

func FuseBehavioralSignals(processWrite RateMetrics, paramDrift RateMetrics) (anomalous bool, sev Severity, burst BurstSnapshot) {
	sev = SeverityNone

	if processWrite.Anomaly.IsAnomalous {
		anomalous = true
		sev = maxSeverity(sev, processWrite.Anomaly.Severity)
	}
	if paramDrift.Anomaly.IsAnomalous {
		anomalous = true
		sev = maxSeverity(sev, paramDrift.Anomaly.Severity)
	}

	burst = processWrite.Burst
	if paramDrift.Burst.Score > burst.Score {
		burst = paramDrift.Burst
	} else if paramDrift.Burst.Score == burst.Score {
		burst.Severity = maxSeverity(burst.Severity, paramDrift.Burst.Severity)
	}

	return anomalous, sev, burst
}

type rateSeriesConfig struct {
	alpha           float64
	minInterval     time.Duration
	anomaly         AnomalyConfig
	burst           BurstConfig
	maxEntries      int
	entryTTL        time.Duration
	cleanupInterval time.Duration
}

type rateSeries struct {
	lastEvent atomic.Int64
	lastSeen  atomic.Int64

	ewma    *EWMAStat
	running *RunningStat
	burst   *BurstDetector
}

func newRateSeries(cfg rateSeriesConfig) *rateSeries {
	return &rateSeries{
		ewma:    NewEWMAStat(cfg.alpha),
		running: &RunningStat{},
		burst:   NewBurstDetector(cfg.burst),
	}
}

type rateSeriesManager struct {
	cfg rateSeriesConfig

	series sync.Map // string -> *rateSeries
	count  atomic.Int64

	lastCleanup atomic.Int64
}

func newRateSeriesManager(cfg rateSeriesConfig) *rateSeriesManager {
	return &rateSeriesManager{cfg: cfg}
}

func (m *rateSeriesManager) Observe(key string, now time.Time) RateMetrics {
	if key == "" {
		return RateMetrics{}
	}

	m.maybeCleanup(now)

	val, loaded := m.series.Load(key)
	if !loaded {
		created := newRateSeries(m.cfg)
		actual, loaded2 := m.series.LoadOrStore(key, created)
		if !loaded2 {
			m.count.Add(1)
			val = created
		} else {
			val = actual
		}
	}

	s := val.(*rateSeries)
	s.lastSeen.Store(now.UnixNano())

	burst := s.burst.Add(now)
	prev := s.lastEvent.Swap(now.UnixNano())
	if prev == 0 {
		return RateMetrics{Burst: burst}
	}

	dt := now.Sub(time.Unix(0, prev))
	if dt < m.cfg.minInterval {
		dt = m.cfg.minInterval
	}
	if dt <= 0 {
		// Fall back to minInterval if clocks jitter.
		dt = m.cfg.minInterval
	}

	currentRate := 1.0 / dt.Seconds()

	baseline := s.running.Snapshot()
	z := ComputeZScore(currentRate, baseline.Mean, baseline.StdDev)
	anomaly := ClassifyZScore(z, m.cfg.anomaly, baseline.Count)

	s.running.Update(currentRate)
	ewma := s.ewma.Update(currentRate)

	return RateMetrics{
		Current:     currentRate,
		EWMA:        ewma,
		Mean:        baseline.Mean,
		Variance:    baseline.Variance,
		StdDev:      baseline.StdDev,
		SampleCount: baseline.Count,
		Anomaly:     anomaly,
		Burst:       burst,
	}
}

func (m *rateSeriesManager) maybeCleanup(now time.Time) {
	interval := m.cfg.cleanupInterval
	if interval <= 0 {
		return
	}

	last := m.lastCleanup.Load()
	if last != 0 {
		if now.Sub(time.Unix(0, last)) < interval {
			return
		}
	}
	if !m.lastCleanup.CompareAndSwap(last, now.UnixNano()) {
		return
	}

	// TTL cleanup.
	if m.cfg.entryTTL > 0 {
		cutoff := now.Add(-m.cfg.entryTTL).UnixNano()
		m.series.Range(func(k, v any) bool {
			s := v.(*rateSeries)
			if s.lastSeen.Load() < cutoff {
				m.series.Delete(k)
				m.count.Add(-1)
			}
			return true
		})
	}

	// Hard cap (only configured for process series).
	if m.cfg.maxEntries > 0 {
		excess := m.count.Load() - int64(m.cfg.maxEntries)
		if excess > 0 {
			m.series.Range(func(k, v any) bool {
				if excess <= 0 {
					return false
				}
				m.series.Delete(k)
				m.count.Add(-1)
				excess--
				return true
			})
		}
	}
}
