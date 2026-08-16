const METRICS_URL = import.meta.env.VITE_METRICS_URL || 'http://localhost:9090/metrics'

export interface MetricsData {
  latency: {
    p50: number
    p95: number
    p99: number
  }
  cacheHitRate: number
  throughput: number
  expansionDepth: number[]
  activeConnections: number
  circuitBreakerState: 'closed' | 'open' | 'half_open'
}

export async function fetchMetrics(): Promise<MetricsData> {
  try {
    const response = await fetch(METRICS_URL)
    const text = await response.text()
    
    // Parse Prometheus metrics format
    const lines = text.split('\n')
    const metrics: Partial<MetricsData> = {
      latency: { p50: 0, p95: 0, p99: 0 },
      cacheHitRate: 0,
      throughput: 0,
      expansionDepth: [],
      activeConnections: 0,
      circuitBreakerState: 'closed',
    }

    // Simple parsing (in production, use a proper Prometheus parser)
    lines.forEach((line) => {
      if (line.startsWith('zenith_request_duration_seconds')) {
        // Parse latency metrics
      }
      if (line.startsWith('zenith_cache_hits_total')) {
        // Parse cache metrics
      }
      if (line.startsWith('zenith_active_connections')) {
        const match = line.match(/zenith_active_connections\s+(\d+)/)
        if (match) {
          metrics.activeConnections = parseInt(match[1], 10)
        }
      }
    })

    return metrics as MetricsData
  } catch (error) {
    console.warn('Failed to fetch metrics:', error)
    // Return mock data for development
    return {
      latency: { p50: 5, p95: 15, p99: 25 },
      cacheHitRate: 72,
      throughput: 2340,
      expansionDepth: [0, 1, 2, 3, 4, 5],
      activeConnections: 5,
      circuitBreakerState: 'closed',
    }
  }
}

