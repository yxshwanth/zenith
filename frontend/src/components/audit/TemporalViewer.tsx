import { useState } from 'react'
import { format } from 'date-fns'

export default function TemporalViewer() {
  const [selectedDate, setSelectedDate] = useState(new Date().toISOString().split('T')[0])
  const [selectedTime, setSelectedTime] = useState('12:00')

  return (
    <div className="space-y-4">
      <div className="card">
        <h1 className="text-2xl font-bold mb-4">Temporal Audit Trail</h1>
        <p className="text-gray-600 dark:text-gray-400 mb-6">
          View permission state at any point in time. Navigate through history to see
          how permissions changed over time.
        </p>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <div>
            <label className="block text-sm font-medium mb-2">Date</label>
            <input
              type="date"
              value={selectedDate}
              onChange={(e) => setSelectedDate(e.target.value)}
              className="input"
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Time</label>
            <input
              type="time"
              value={selectedTime}
              onChange={(e) => setSelectedTime(e.target.value)}
              className="input"
            />
          </div>
        </div>

        <button className="btn btn-primary">Query at Time</button>
      </div>

      <div className="card">
        <h2 className="text-lg font-semibold mb-4">
          Permissions at {format(new Date(`${selectedDate}T${selectedTime}`), 'PPpp')}
        </h2>
        <div className="text-gray-600 dark:text-gray-400">
          <p>Note: This feature requires the temporal tables migration to be applied.</p>
          <p className="mt-2">
            The QueryAtTime API endpoint will be used to fetch permissions at the
            specified timestamp.
          </p>
        </div>
      </div>
    </div>
  )
}

