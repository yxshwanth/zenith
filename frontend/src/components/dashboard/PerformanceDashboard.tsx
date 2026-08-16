import { useState } from 'react'
import { useMetrics } from '../../hooks/useMetrics'
import LatencyChart from './LatencyChart'
import CacheMetrics from './CacheMetrics'
import ExpansionDepthChart from './ExpansionDepthChart'

export default function PerformanceDashboard() {
  const [refetchInterval, setRefetchInterval] = useState(5000)
  const { data: metrics, isLoading, error } = useMetrics(refetchInterval)

  if (isLoading) {
    return (
      <div className="card">
        <div className="flex items-center justify-center h-96">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600"></div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="card">
        <div className="text-red-600 dark:text-red-400">
          Error loading metrics: {error instanceof Error ? error.message : 'Unknown error'}
        </div>
      </div>
    )
  }

  if (!metrics) {
    return null
  }

  return (
    <div className="space-y-4">
      <div className="card">
        <div className="flex justify-between items-center mb-4">
          <div>
            <h1 className="text-2xl font-bold">Performance Dashboard</h1>
            <p className="text-gray-600 dark:text-gray-400 mt-1">
              Real-time performance metrics and monitoring
            </p>
          </div>
          <div className="flex items-center space-x-2">
            <label className="text-sm">Auto-refresh:</label>
            <select
              value={refetchInterval}
              onChange={(e) => setRefetchInterval(Number(e.target.value))}
              className="input text-sm"
            >
              <option value={0}>Off</option>
              <option value={1000}>1s</option>
              <option value={5000}>5s</option>
              <option value={10000}>10s</option>
              <option value={30000}>30s</option>
            </select>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="card">
          <div className="text-sm text-gray-600 dark:text-gray-400">P50 Latency</div>
          <div className="text-2xl font-bold">{metrics.latency.p50.toFixed(2)}ms</div>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600 dark:text-gray-400">P95 Latency</div>
          <div className="text-2xl font-bold">{metrics.latency.p95.toFixed(2)}ms</div>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600 dark:text-gray-400">P99 Latency</div>
          <div className="text-2xl font-bold">{metrics.latency.p99.toFixed(2)}ms</div>
        </div>
        <div className="card">
          <div className="text-sm text-gray-600 dark:text-gray-400">Throughput</div>
          <div className="text-2xl font-bold">{metrics.throughput} req/s</div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <LatencyChart metrics={metrics} />
        <CacheMetrics metrics={metrics} />
      </div>

      <ExpansionDepthChart metrics={metrics} />

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Active Connections</h2>
          <div className="text-3xl font-bold">{metrics.activeConnections}/25</div>
          <div className="mt-2 w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
            <div
              className="bg-primary-600 h-2 rounded-full"
              style={{ width: `${(metrics.activeConnections / 25) * 100}%` }}
            ></div>
          </div>
        </div>
        <div className="card">
          <h2 className="text-lg font-semibold mb-4">Circuit Breaker</h2>
          <div
            className={`text-2xl font-bold ${
              metrics.circuitBreakerState === 'closed'
                ? 'text-green-600 dark:text-green-400'
                : metrics.circuitBreakerState === 'open'
                ? 'text-red-600 dark:text-red-400'
                : 'text-yellow-600 dark:text-yellow-400'
            }`}
          >
            {metrics.circuitBreakerState.toUpperCase()}
          </div>
        </div>
      </div>
    </div>
  )
}

