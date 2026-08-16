import type { RelationTuple } from '../../types/api'

interface TupleTableProps {
  tuples: RelationTuple[]
  onDelete: (tuple: RelationTuple) => void
}

export default function TupleTable({ tuples, onDelete }: TupleTableProps) {
  if (tuples.length === 0) {
    return (
      <div className="card">
        <p className="text-gray-600 dark:text-gray-400 text-center py-8">
          No tuples found. Create one to get started.
        </p>
      </div>
    )
  }

  return (
    <div className="card">
      <h2 className="text-lg font-semibold mb-4">Tuples ({tuples.length})</h2>
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
          <thead className="bg-gray-50 dark:bg-gray-700">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Subject
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Relation
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Object
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
            {tuples.map((tuple, idx) => (
              <tr key={idx}>
                <td className="px-4 py-3 whitespace-nowrap text-sm">
                  {tuple.subject_namespace}:{tuple.subject_id}
                  {tuple.subject_relation && `#${tuple.subject_relation}`}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm">
                  {tuple.relation}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm">
                  {tuple.namespace}:{tuple.object_id}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm">
                  <button
                    onClick={() => onDelete(tuple)}
                    className="text-red-600 dark:text-red-400 hover:text-red-800 dark:hover:text-red-300"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

