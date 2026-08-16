import { useState, useEffect } from 'react'

interface CheckHistoryItem {
  subject: string
  relation: string
  object: string
  allowed: boolean
  timestamp: Date
  latency?: number
}

export default function CheckHistory() {
  const [history, setHistory] = useState<CheckHistoryItem[]>([])

  useEffect(() => {
    const stored = localStorage.getItem('checkHistory')
    if (stored) {
      try {
        const parsed = JSON.parse(stored).map((item: any) => ({
          ...item,
          timestamp: new Date(item.timestamp),
        }))
        setHistory(parsed)
      } catch (e) {
        // Ignore parse errors
      }
    }
  }, [])

  if (history.length === 0) {
    return null
  }

  return (
    <div className="card">
      <h2 className="text-lg font-semibold mb-4">Recent Checks</h2>
      <div className="space-y-2">
        {history.slice(0, 5).map((item, idx) => (
          <div
            key={idx}
            className="flex items-center justify-between p-2 bg-gray-50 dark:bg-gray-700 rounded"
          >
            <div className="flex items-center space-x-2">
              <span>{item.allowed ? '✅' : '❌'}</span>
              <span className="text-sm">
                {item.subject} → {item.relation} → {item.object}
              </span>
            </div>
            {item.latency && (
              <span className="text-xs text-gray-500 dark:text-gray-400">
                {item.latency.toFixed(2)}ms
              </span>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

