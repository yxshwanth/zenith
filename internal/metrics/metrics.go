package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for Zenith
var (
	// Request counters by operation type and status
	RequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zenith_requests_total",
			Help: "Total number of requests by operation and status",
		},
		[]string{"operation", "status"},
	)

	// Request duration histograms
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zenith_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets, // Default buckets: .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
		},
		[]string{"operation"},
	)

	// Cache metrics
	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zenith_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	CacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zenith_cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	// Expansion depth distribution
	ExpansionDepth = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zenith_expansion_depth",
			Help:    "Distribution of expansion depths",
			Buckets: []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20},
		},
		[]string{"operation"},
	)

	// Database query metrics
	DatabaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zenith_database_query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
		[]string{"operation"},
	)

	// Active connections
	ActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "zenith_active_connections",
			Help: "Number of active database connections",
		},
	)

	// Batch check metrics
	BatchCheckSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "zenith_batch_check_size",
			Help:    "Distribution of batch check sizes (number of checks per batch)",
			Buckets: []float64{1, 5, 10, 15, 20, 25, 30},
		},
	)

	// SLO metrics
	SLOCheckLatencyP99 = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "zenith_slo_check_latency_p99_seconds",
			Help: "99th percentile latency for check operations in seconds",
		},
	)

	SLOErrorBudgetRemaining = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "zenith_slo_error_budget_remaining_percent",
			Help: "Remaining error budget percentage",
		},
	)

	SLOAvailabilityPercentage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "zenith_slo_availability_percentage",
			Help: "Service availability percentage",
		},
	)

	SLOCacheHitRate = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "zenith_slo_cache_hit_rate_percent",
			Help: "Cache hit rate percentage",
		},
	)

	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zenith_circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=open, 2=half_open)",
		},
		[]string{"operation"},
	)

	CircuitBreakerFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zenith_circuit_breaker_failures_total",
			Help: "Total number of circuit breaker failures",
		},
		[]string{"operation"},
	)

	StaleCacheServed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zenith_stale_cache_served_total",
			Help: "Total number of stale cache entries served",
		},
		[]string{"operation"},
	)
)

// RecordRequest records a request with operation type and status
func RecordRequest(operation, status string) {
	RequestTotal.WithLabelValues(operation, status).Inc()
}

// RecordRequestDuration records the duration of a request
func RecordRequestDuration(operation string, duration time.Duration) {
	RequestDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordCacheHit records a cache hit
func RecordCacheHit(cacheType string) {
	CacheHits.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss records a cache miss
func RecordCacheMiss(cacheType string) {
	CacheMisses.WithLabelValues(cacheType).Inc()
}

// RecordExpansionDepth records the depth of an expansion operation
func RecordExpansionDepth(operation string, depth int) {
	ExpansionDepth.WithLabelValues(operation).Observe(float64(depth))
}

// RecordDatabaseQuery records a database query duration
func RecordDatabaseQuery(operation string, duration time.Duration) {
	DatabaseQueryDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// SetActiveConnections sets the number of active database connections
func SetActiveConnections(count float64) {
	ActiveConnections.Set(count)
}

// RecordBatchCheckSize records the size of a batch check operation
func RecordBatchCheckSize(size int) {
	BatchCheckSize.Observe(float64(size))
}

// SetSLOCheckLatencyP99 sets the 99th percentile latency for check operations
func SetSLOCheckLatencyP99(seconds float64) {
	SLOCheckLatencyP99.Set(seconds)
}

// SetSLOErrorBudgetRemaining sets the remaining error budget percentage
func SetSLOErrorBudgetRemaining(percent float64) {
	SLOErrorBudgetRemaining.Set(percent)
}

// SetSLOAvailabilityPercentage sets the availability percentage
func SetSLOAvailabilityPercentage(percent float64) {
	SLOAvailabilityPercentage.Set(percent)
}

// SetSLOCacheHitRate sets the cache hit rate percentage
func SetSLOCacheHitRate(percent float64) {
	SLOCacheHitRate.Set(percent)
}

// SetCircuitBreakerState sets the circuit breaker state
func SetCircuitBreakerState(operation string, state float64) {
	CircuitBreakerState.WithLabelValues(operation).Set(state)
}

// RecordCircuitBreakerFailure records a circuit breaker failure
func RecordCircuitBreakerFailure(operation string) {
	CircuitBreakerFailures.WithLabelValues(operation).Inc()
}

// RecordStaleCacheServed records a stale cache entry being served
func RecordStaleCacheServed(operation string) {
	StaleCacheServed.WithLabelValues(operation).Inc()
}

