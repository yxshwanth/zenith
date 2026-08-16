import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import type { MetricsData } from '../../services/metricsService'

interface LatencyChartProps {
  metrics: MetricsData
}

export default function LatencyChart({ metrics }: LatencyChartProps) {
  const data = [
    {
      name: 'P50',
      value: metrics.latency.p50,
    },
    {
      name: 'P95',
      value: metrics.latency.p95,
    },
    {
      name: 'P99',
      value: metrics.latency.p99,
    },
  ]

  return (
    <div className="card">
      <h2 className="text-lg font-semibold mb-4">Latency Distribution</h2>
      <ResponsiveContainer width="100%" height={300}>
        <LineChart data={data}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="name" />
          <YAxis label={{ value: 'Latency (ms)', angle: -90, position: 'insideLeft' }} />
          <Tooltip />
          <Legend />
          <Line type="monotone" dataKey="value" stroke="#3b82f6" strokeWidth={2} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}

