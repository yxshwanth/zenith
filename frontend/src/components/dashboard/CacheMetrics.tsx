import { PieChart, Pie, Cell, ResponsiveContainer, Legend, Tooltip } from 'recharts'
import type { MetricsData } from '../../services/metricsService'

interface CacheMetricsProps {
  metrics: MetricsData
}

export default function CacheMetrics({ metrics }: CacheMetricsProps) {
  const data = [
    { name: 'Cache Hits', value: metrics.cacheHitRate },
    { name: 'Cache Misses', value: 100 - metrics.cacheHitRate },
  ]

  const COLORS = ['#10b981', '#ef4444']

  return (
    <div className="card">
      <h2 className="text-lg font-semibold mb-4">Cache Hit Rate</h2>
      <div className="text-center mb-4">
        <div className="text-4xl font-bold text-primary-600 dark:text-primary-400">
          {metrics.cacheHitRate.toFixed(1)}%
        </div>
      </div>
      <ResponsiveContainer width="100%" height={300}>
        <PieChart>
          <Pie
            data={data}
            cx="50%"
            cy="50%"
            labelLine={false}
            label={({ name, percent }) => `${name}: ${(percent * 100).toFixed(0)}%`}
            outerRadius={80}
            fill="#8884d8"
            dataKey="value"
          >
            {data.map((entry, index) => (
              <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
            ))}
          </Pie>
          <Tooltip />
          <Legend />
        </PieChart>
      </ResponsiveContainer>
    </div>
  )
}

