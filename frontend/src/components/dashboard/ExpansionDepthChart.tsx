import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import type { MetricsData } from '../../services/metricsService'

interface ExpansionDepthChartProps {
  metrics: MetricsData
}

export default function ExpansionDepthChart({ metrics }: ExpansionDepthChartProps) {
  const data = metrics.expansionDepth.map((count, depth) => ({
    depth,
    count,
  }))

  return (
    <div className="card">
      <h2 className="text-lg font-semibold mb-4">Expansion Depth Distribution</h2>
      <ResponsiveContainer width="100%" height={300}>
        <BarChart data={data}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="depth" label={{ value: 'Depth', position: 'insideBottom', offset: -5 }} />
          <YAxis label={{ value: 'Count', angle: -90, position: 'insideLeft' }} />
          <Tooltip />
          <Legend />
          <Bar dataKey="count" fill="#3b82f6" />
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}

