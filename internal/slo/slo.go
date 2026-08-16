package slo

import (
	"context"
	"sync"
	"time"
)

// SLO represents a Service Level Objective
type SLO struct {
	Name        string
	Objective   float64 // Target percentage (e.g., 99.0 for 99%)
	ErrorBudget float64 // Error budget (e.g., 0.01 for 1%)
	Window      time.Duration
}

// SLODefinition holds SLO configuration
type SLODefinition struct {
	// Check latency SLO: 99% of checks < 50ms
	CheckLatencyP99 SLO

	// Cache hit rate SLO: > 70%
	CacheHitRate SLO

	// Error rate SLO: < 0.1%
	ErrorRate SLO

	// Availability SLO: > 99.9%
	Availability SLO
}

// DefaultSLODefinitions returns default SLO definitions
func DefaultSLODefinitions() SLODefinition {
	return SLODefinition{
		CheckLatencyP99: SLO{
			Name:        "check_latency_p99",
			Objective:   99.0,
			ErrorBudget: 0.01,
			Window:      5 * time.Minute,
		},
		CacheHitRate: SLO{
			Name:        "cache_hit_rate",
			Objective:   70.0,
			ErrorBudget: 0.30, // 30% error budget
			Window:      5 * time.Minute,
		},
		ErrorRate: SLO{
			Name:        "error_rate",
			Objective:   99.9, // 0.1% error rate
			ErrorBudget: 0.001,
			Window:      5 * time.Minute,
		},
		Availability: SLO{
			Name:        "availability",
			Objective:   99.9,
			ErrorBudget: 0.001,
			Window:      1 * time.Hour,
		},
	}
}

// SLOTracker tracks SLO metrics and error budgets
type SLOTracker struct {
	definitions SLODefinition
	mu          sync.RWMutex

	// Metrics windows
	latencyWindow    []time.Duration
	cacheHits        int64
	cacheMisses      int64
	errors           int64
	totalRequests    int64
	lastWindowReset  time.Time
}

// NewSLOTracker creates a new SLO tracker
func NewSLOTracker(definitions SLODefinition) *SLOTracker {
	return &SLOTracker{
		definitions:     definitions,
		latencyWindow:   make([]time.Duration, 0, 10000),
		lastWindowReset: time.Now(),
	}
}

// RecordLatency records a request latency
func (t *SLOTracker) RecordLatency(duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.latencyWindow = append(t.latencyWindow, duration)
	// Keep only recent latencies (last 5 minutes worth)
	if len(t.latencyWindow) > 10000 {
		t.latencyWindow = t.latencyWindow[len(t.latencyWindow)-10000:]
	}
}

// RecordCacheHit records a cache hit
func (t *SLOTracker) RecordCacheHit() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cacheHits++
}

// RecordCacheMiss records a cache miss
func (t *SLOTracker) RecordCacheMiss() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cacheMisses++
}

// RecordError records an error
func (t *SLOTracker) RecordError() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errors++
	t.totalRequests++
}

// RecordRequest records a successful request
func (t *SLOTracker) RecordRequest() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totalRequests++
}

// GetCheckLatencyP99 returns the 99th percentile latency
func (t *SLOTracker) GetCheckLatencyP99() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.latencyWindow) == 0 {
		return 0
	}

	// Calculate p99
	sorted := make([]time.Duration, len(t.latencyWindow))
	copy(sorted, t.latencyWindow)
	
	// Simple sort (bubble sort for small arrays, or use sort.Slice)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := int(float64(len(sorted)) * 0.99)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

// GetCacheHitRate returns the cache hit rate percentage
func (t *SLOTracker) GetCacheHitRate() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	total := t.cacheHits + t.cacheMisses
	if total == 0 {
		return 0
	}

	return float64(t.cacheHits) / float64(total) * 100.0
}

// GetErrorRate returns the error rate percentage
func (t *SLOTracker) GetErrorRate() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.totalRequests == 0 {
		return 0
	}

	return float64(t.errors) / float64(t.totalRequests) * 100.0
}

// GetErrorBudgetConsumed returns the percentage of error budget consumed
func (t *SLOTracker) GetErrorBudgetConsumed() float64 {
	// For check latency: calculate how many requests exceed 50ms
	t.mu.RLock()
	exceeded := 0
	for _, lat := range t.latencyWindow {
		if lat > 50*time.Millisecond {
			exceeded++
		}
	}
	totalLatency := len(t.latencyWindow)
	t.mu.RUnlock()

	if totalLatency == 0 {
		return 0
	}

	// Error budget is 1% (0.01)
	// If more than 1% exceed 50ms, we're consuming error budget
	exceedRate := float64(exceeded) / float64(totalLatency)
	if exceedRate <= 0.01 {
		return 0
	}

	// Calculate consumed budget
	return (exceedRate - 0.01) / 0.01 * 100.0
}

// ResetWindow resets the metrics window
func (t *SLOTracker) ResetWindow() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.latencyWindow = t.latencyWindow[:0]
	t.cacheHits = 0
	t.cacheMisses = 0
	t.errors = 0
	t.totalRequests = 0
	t.lastWindowReset = time.Now()
}

// StartWindowReset starts a goroutine that resets the window periodically
func (t *SLOTracker) StartWindowReset(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				t.ResetWindow()
			}
		}
	}()
}

// Stats returns current SLO statistics
type Stats struct {
	CheckLatencyP99      time.Duration
	CacheHitRate         float64
	ErrorRate            float64
	ErrorBudgetConsumed  float64
	TotalRequests        int64
	CacheHits            int64
	CacheMisses          int64
	Errors               int64
}

// GetStats returns current SLO statistics
func (t *SLOTracker) GetStats() Stats {
	return Stats{
		CheckLatencyP99:     t.GetCheckLatencyP99(),
		CacheHitRate:        t.GetCacheHitRate(),
		ErrorRate:           t.GetErrorRate(),
		ErrorBudgetConsumed: t.GetErrorBudgetConsumed(),
		TotalRequests:        t.totalRequests,
		CacheHits:           t.cacheHits,
		CacheMisses:         t.cacheMisses,
		Errors:              t.errors,
	}
}

