import { useState } from 'react'
import { useBatchCheck } from '../../hooks/useZenithAPI'
import type { BatchCheckRequest, CheckRequest } from '../../types/api'
import BatchResultsTable from './BatchResultsTable'

const sampleBatch: CheckRequest[] = [
  {
    subject_namespace: 'user',
    subject_id: 'alice',
    namespace: 'doc',
    object_id: 'doc_1',
    relation: 'viewer',
  },
  {
    subject_namespace: 'user',
    subject_id: 'bob',
    namespace: 'doc',
    object_id: 'doc_2',
    relation: 'editor',
  },
  {
    subject_namespace: 'user',
    subject_id: 'charlie',
    namespace: 'doc',
    object_id: 'doc_1',
    relation: 'viewer',
  },
]

export default function BatchChecker() {
  const [jsonInput, setJsonInput] = useState(
    JSON.stringify({ requests: sampleBatch }, null, 2)
  )
  const [batchRequest, setBatchRequest] = useState<BatchCheckRequest | null>(null)
  const { data: batchResult, isLoading } = useBatchCheck(
    batchRequest!,
    !!batchRequest
  )

  const handleLoadSample = () => {
    setJsonInput(JSON.stringify({ requests: sampleBatch }, null, 2))
  }

  const handleCheck = () => {
    try {
      const parsed = JSON.parse(jsonInput)
      if (!parsed.requests || !Array.isArray(parsed.requests)) {
        alert('Invalid format. Expected: { "requests": [...] }')
        return
      }

      if (parsed.requests.length > 30) {
        alert('Maximum 30 checks allowed per batch')
        return
      }

      setBatchRequest({
        requests: parsed.requests,
        required_zookie: parsed.required_zookie,
      })
    } catch (e) {
      alert('Invalid JSON: ' + (e instanceof Error ? e.message : 'Unknown error'))
    }
  }

  const allowedCount =
    batchResult?.results.filter((r) => r.allowed).length || 0
  const totalCount = batchResult?.results.length || 0
  const efficiencyGain =
    batchResult?.total_latency && totalCount > 0
      ? ((batchResult.total_latency / totalCount) * totalCount) /
        batchResult.total_latency
      : 0

  return (
    <div className="space-y-4">
      <div className="card">
        <h1 className="text-2xl font-bold mb-4">Batch Permission Checker</h1>
        <p className="text-gray-600 dark:text-gray-400 mb-6">
          Check multiple permissions in a single request. Demonstrates the efficiency
          of batch operations.
        </p>

        <div className="mb-4 flex space-x-2">
          <button onClick={handleLoadSample} className="btn btn-secondary">
            Load Sample
          </button>
          <button
            onClick={handleCheck}
            disabled={isLoading}
            className="btn btn-primary"
          >
            {isLoading ? 'Checking...' : 'Check All'}
          </button>
        </div>

        <textarea
          value={jsonInput}
          onChange={(e) => setJsonInput(e.target.value)}
          className="input font-mono text-sm"
          rows={15}
          placeholder='{ "requests": [...] }'
        />
      </div>

      {batchResult && (
        <>
          <div className="card">
            <h2 className="text-lg font-semibold mb-4">Summary</h2>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div>
                <div className="text-sm text-gray-600 dark:text-gray-400">Allowed</div>
                <div className="text-2xl font-bold text-green-600 dark:text-green-400">
                  {allowedCount}
                </div>
              </div>
              <div>
                <div className="text-sm text-gray-600 dark:text-gray-400">Denied</div>
                <div className="text-2xl font-bold text-red-600 dark:text-red-400">
                  {totalCount - allowedCount}
                </div>
              </div>
              <div>
                <div className="text-sm text-gray-600 dark:text-gray-400">
                  Total Latency
                </div>
                <div className="text-2xl font-bold">
                  {batchResult.total_latency?.toFixed(2)}ms
                </div>
              </div>
              <div>
                <div className="text-sm text-gray-600 dark:text-gray-400">
                  Efficiency Gain
                </div>
                <div className="text-2xl font-bold text-primary-600 dark:text-primary-400">
                  {efficiencyGain.toFixed(1)}x
                </div>
              </div>
            </div>
          </div>

          <BatchResultsTable
            requests={batchRequest!.requests}
            results={batchResult.results}
            totalLatency={batchResult.total_latency || 0}
          />
        </>
      )}
    </div>
  )
}

