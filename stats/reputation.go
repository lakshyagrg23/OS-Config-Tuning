package stats

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type ReputationConfig struct {
	InitialScore         float64       `yaml:"initial_score"`
	MinScore             float64       `yaml:"min_score"`
	MaxScore             float64       `yaml:"max_score"`
	NeutralScore         float64       `yaml:"neutral_score"`
	HalfLife             time.Duration `yaml:"half_life"`
	RewardStable         float64       `yaml:"reward_stable"`
	PenaltyDrift         float64       `yaml:"penalty_drift"`
	PenaltyAlert         float64       `yaml:"penalty_alert"`
	PenaltyRemed         float64       `yaml:"penalty_remediate"`
	PenaltyCooldown      float64       `yaml:"penalty_cooldown"`
	PenaltyConflict      float64       `yaml:"penalty_conflict"`
	PenaltyAnomalyMedium float64       `yaml:"penalty_anomaly_medium"`
	PenaltyAnomalyHigh   float64       `yaml:"penalty_anomaly_high"`
	PenaltyBurstMedium   float64       `yaml:"penalty_burst_medium"`
	PenaltyBurstHigh     float64       `yaml:"penalty_burst_high"`

	MaxEntries      int           `yaml:"max_entries"`
	EntryTTL        time.Duration `yaml:"entry_ttl"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
}

func (c ReputationConfig) withDefaults() ReputationConfig {
	out := c
	if out.InitialScore <= 0 {
		out.InitialScore = 0.5
	}
	if out.MinScore < 0 {
		out.MinScore = 0
	}
	if out.MaxScore <= 0 {
		out.MaxScore = 1
	}
	if out.NeutralScore <= 0 {
		out.NeutralScore = 0.5
	}
	if out.HalfLife <= 0 {
		out.HalfLife = 30 * time.Minute
	}

	if out.RewardStable <= 0 {
		out.RewardStable = 0.001
	}
	if out.PenaltyDrift <= 0 {
		out.PenaltyDrift = 0.02
	}
	if out.PenaltyAlert <= 0 {
		out.PenaltyAlert = 0.01
	}
	if out.PenaltyRemed <= 0 {
		out.PenaltyRemed = 0.03
	}
	if out.PenaltyCooldown <= 0 {
		out.PenaltyCooldown = 0.005
	}
	if out.PenaltyConflict <= 0 {
		out.PenaltyConflict = 0.01
	}
	if out.PenaltyAnomalyMedium <= 0 {
		out.PenaltyAnomalyMedium = 0.01
	}
	if out.PenaltyAnomalyHigh <= 0 {
		out.PenaltyAnomalyHigh = 0.02
	}
	if out.PenaltyBurstMedium <= 0 {
		out.PenaltyBurstMedium = 0.01
	}
	if out.PenaltyBurstHigh <= 0 {
		out.PenaltyBurstHigh = 0.02
	}

	if out.MaxEntries <= 0 {
		out.MaxEntries = 2048
	}
	if out.EntryTTL <= 0 {
		out.EntryTTL = 30 * time.Minute
	}
	if out.CleanupInterval <= 0 {
		out.CleanupInterval = 1 * time.Minute
	}

	if out.MaxScore < out.MinScore {
		out.MaxScore = out.MinScore
	}
	if out.NeutralScore < out.MinScore {
		out.NeutralScore = out.MinScore
	}
	if out.NeutralScore > out.MaxScore {
		out.NeutralScore = out.MaxScore
	}
	if out.InitialScore < out.MinScore {
		out.InitialScore = out.MinScore
	}
	if out.InitialScore > out.MaxScore {
		out.InitialScore = out.MaxScore
	}

	return out
}

type Reputation struct {
	Score       float64
	LastUpdated time.Time
}

func (r *Reputation) Decay(cfg ReputationConfig, now time.Time) {
	cfg = cfg.withDefaults()
	if r.LastUpdated.IsZero() {
		r.LastUpdated = now
		return
	}
	if now.Before(r.LastUpdated) {
		return
	}

	dt := now.Sub(r.LastUpdated)
	if dt <= 0 {
		return
	}

	h := cfg.HalfLife
	if h <= 0 {
		r.LastUpdated = now
		return
	}

	factor := math.Pow(0.5, dt.Seconds()/h.Seconds())
	r.Score = cfg.NeutralScore + (r.Score-cfg.NeutralScore)*factor
	r.LastUpdated = now
}

func (r *Reputation) Increase(delta float64, cfg ReputationConfig, now time.Time) {
	cfg = cfg.withDefaults()
	r.Decay(cfg, now)
	r.Score += delta
	if r.Score > cfg.MaxScore {
		r.Score = cfg.MaxScore
	}
	if r.Score < cfg.MinScore {
		r.Score = cfg.MinScore
	}
	r.LastUpdated = now
}

func (r *Reputation) Decrease(delta float64, cfg ReputationConfig, now time.Time) {
	cfg = cfg.withDefaults()
	r.Decay(cfg, now)
	r.Score -= delta
	if r.Score < cfg.MinScore {
		r.Score = cfg.MinScore
	}
	if r.Score > cfg.MaxScore {
		r.Score = cfg.MaxScore
	}
	r.LastUpdated = now
}

func (r *Reputation) GetScore(cfg ReputationConfig, now time.Time) float64 {
	cfg = cfg.withDefaults()
	r.Decay(cfg, now)
	if r.Score < cfg.MinScore {
		r.Score = cfg.MinScore
	}
	if r.Score > cfg.MaxScore {
		r.Score = cfg.MaxScore
	}
	return r.Score
}

type ReputationSnapshot struct {
	Score               float64
	StableFor           time.Duration
	AnomalyCount        int64
	RemediationAssocCnt int64
	ConflictAssocCnt    int64
}

type ReputationUpdate struct {
	WasDrift         bool
	FinalAction      string
	CooldownApplied  bool
	ConflictDetected bool
	AnomalySeverity  Severity
	BurstSeverity    Severity
}

type reputationEntry struct {
	lastSeen atomic.Int64

	mu          sync.Mutex
	rep         Reputation
	stableSince time.Time
	anomalyCnt  int64
	remedCnt    int64
	conflictCnt int64
}

type ReputationManager struct {
	cfg ReputationConfig

	mu          sync.RWMutex
	entries     map[string]*reputationEntry
	lastCleanup atomic.Int64
}

func NewReputationManager(cfg ReputationConfig) *ReputationManager {
	cfg = cfg.withDefaults()
	return &ReputationManager{
		cfg:     cfg,
		entries: make(map[string]*reputationEntry),
	}
}

func (m *ReputationManager) Get(process string, now time.Time) ReputationSnapshot {
	if process == "" {
		return ReputationSnapshot{}
	}

	entry := m.getOrCreate(process, now)
	entry.lastSeen.Store(now.UnixNano())

	entry.mu.Lock()
	defer entry.mu.Unlock()

	score := entry.rep.GetScore(m.cfg, now)
	stableFor := time.Duration(0)
	if !entry.stableSince.IsZero() {
		stableFor = now.Sub(entry.stableSince)
	}

	return ReputationSnapshot{
		Score:               score,
		StableFor:           stableFor,
		AnomalyCount:        entry.anomalyCnt,
		RemediationAssocCnt: entry.remedCnt,
		ConflictAssocCnt:    entry.conflictCnt,
	}
}

func (m *ReputationManager) Update(process string, now time.Time, upd ReputationUpdate) ReputationSnapshot {
	if process == "" {
		return ReputationSnapshot{}
	}

	entry := m.getOrCreate(process, now)
	entry.lastSeen.Store(now.UnixNano())

	entry.mu.Lock()
	defer entry.mu.Unlock()

	scoreBefore := entry.rep.GetScore(m.cfg, now)
	_ = scoreBefore

	stableEvent := !upd.WasDrift && upd.AnomalySeverity != SeverityHigh && upd.BurstSeverity != SeverityHigh
	if entry.stableSince.IsZero() {
		entry.stableSince = now
	}
	if !stableEvent {
		entry.stableSince = now
	}

	// Positive reinforcement for stable behavior.
	if stableEvent {
		entry.rep.Increase(m.cfg.RewardStable, m.cfg, now)
	}

	// Penalize specific outcomes.
	if upd.WasDrift {
		entry.rep.Decrease(m.cfg.PenaltyDrift, m.cfg, now)
	}

	switch upd.FinalAction {
	case "alert":
		entry.rep.Decrease(m.cfg.PenaltyAlert, m.cfg, now)
	case "remediate":
		entry.rep.Decrease(m.cfg.PenaltyRemed, m.cfg, now)
		entry.remedCnt++
	}

	if upd.CooldownApplied {
		entry.rep.Decrease(m.cfg.PenaltyCooldown, m.cfg, now)
	}
	if upd.ConflictDetected {
		entry.rep.Decrease(m.cfg.PenaltyConflict, m.cfg, now)
		entry.conflictCnt++
	}

	switch upd.AnomalySeverity {
	case SeverityHigh:
		entry.anomalyCnt++
		entry.rep.Decrease(m.cfg.PenaltyAnomalyHigh, m.cfg, now)
	case SeverityMedium:
		entry.anomalyCnt++
		entry.rep.Decrease(m.cfg.PenaltyAnomalyMedium, m.cfg, now)
	}

	switch upd.BurstSeverity {
	case SeverityHigh:
		entry.rep.Decrease(m.cfg.PenaltyBurstHigh, m.cfg, now)
	case SeverityMedium:
		entry.rep.Decrease(m.cfg.PenaltyBurstMedium, m.cfg, now)
	}

	score := entry.rep.GetScore(m.cfg, now)
	stableFor := time.Duration(0)
	if !entry.stableSince.IsZero() {
		stableFor = now.Sub(entry.stableSince)
	}

	return ReputationSnapshot{
		Score:               score,
		StableFor:           stableFor,
		AnomalyCount:        entry.anomalyCnt,
		RemediationAssocCnt: entry.remedCnt,
		ConflictAssocCnt:    entry.conflictCnt,
	}
}

func (m *ReputationManager) getOrCreate(process string, now time.Time) *reputationEntry {
	m.maybeCleanup(now)

	m.mu.RLock()
	entry := m.entries[process]
	m.mu.RUnlock()
	if entry != nil {
		return entry
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry = m.entries[process]
	if entry != nil {
		return entry
	}

	entry = &reputationEntry{
		rep: Reputation{
			Score:       m.cfg.InitialScore,
			LastUpdated: now,
		},
		stableSince: now,
	}
	entry.lastSeen.Store(now.UnixNano())
	m.entries[process] = entry
	return entry
}

func (m *ReputationManager) maybeCleanup(now time.Time) {
	cfg := m.cfg.withDefaults()

	last := m.lastCleanup.Load()
	if last != 0 {
		if now.Sub(time.Unix(0, last)) < cfg.CleanupInterval {
			return
		}
	}
	if !m.lastCleanup.CompareAndSwap(last, now.UnixNano()) {
		return
	}

	cutoff := now.Add(-cfg.EntryTTL).UnixNano()

	m.mu.Lock()
	defer m.mu.Unlock()

	for k, v := range m.entries {
		if v.lastSeen.Load() < cutoff {
			delete(m.entries, k)
		}
	}

	// Hard cap: evict arbitrary entries if still over limit.
	if cfg.MaxEntries > 0 && len(m.entries) > cfg.MaxEntries {
		excess := len(m.entries) - cfg.MaxEntries
		for k := range m.entries {
			delete(m.entries, k)
			excess--
			if excess <= 0 {
				break
			}
		}
	}
}
