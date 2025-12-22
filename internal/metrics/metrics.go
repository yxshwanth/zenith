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

