import type { CheckRequest, CheckResult } from '../../types/api'

interface BatchResultsTableProps {
  requests: CheckRequest[]
  results: CheckResult[]
  totalLatency: number
}

export default function BatchResultsTable({
  requests,
  results,
  totalLatency,
}: BatchResultsTableProps) {
  const avgLatency = totalLatency / results.length

  return (
    <div className="card">
      <h2 className="text-lg font-semibold mb-4">Results</h2>
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead className="bg-gray-50 dark:bg-gray-700">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Subject
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Object
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Relation
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Status
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Latency
              </th>
            </tr>
          </thead>
          <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
            {requests.map((req, idx) => {
              const result = results[idx]
              return (
                <tr key={idx}>
                  <td className="px-4 py-3 whitespace-nowrap text-sm">
                    {req.subject_namespace}:{req.subject_id}
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap text-sm">
                    {req.namespace}:{req.object_id}
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap text-sm">
                    {req.relation}
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <span
                      className={`px-2 py-1 text-xs font-semibold rounded ${
                        result.allowed
                          ? 'bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200'
                          : 'bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-200'
                      }`}
                    >
                      {result.allowed ? '✅ ALLOWED' : '❌ DENIED'}
                    </span>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap text-sm">
                    {avgLatency.toFixed(2)}ms
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

