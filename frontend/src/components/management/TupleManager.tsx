import { useState } from 'react'
import { useTuples, useWriteTuple, useDeleteTuple } from '../../hooks/useZenithAPI'
import TupleTable from './TupleTable'
import TupleForm from './TupleForm'

export default function TupleManager() {
  const { data: tuples, isLoading, error } = useTuples()
  const writeMutation = useWriteTuple()
  const deleteMutation = useDeleteTuple()
  const [showForm, setShowForm] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [filterNamespace, setFilterNamespace] = useState('')

  const namespaces = Array.from(
    new Set(tuples?.map((t) => t.namespace) || [])
  ).sort()

  const filteredTuples =
    tuples?.filter((tuple) => {
      const matchesSearch =
        searchTerm === '' ||
        tuple.subject_id.toLowerCase().includes(searchTerm.toLowerCase()) ||
        tuple.object_id.toLowerCase().includes(searchTerm.toLowerCase()) ||
        tuple.relation.toLowerCase().includes(searchTerm.toLowerCase())

      const matchesNamespace =
        filterNamespace === '' || tuple.namespace === filterNamespace

      return matchesSearch && matchesNamespace
    }) || []

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
          Error loading tuples: {error instanceof Error ? error.message : 'Unknown error'}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="card">
        <div className="flex justify-between items-center mb-4">
          <div>
            <h1 className="text-2xl font-bold">Permission Tuple Manager</h1>
            <p className="text-gray-600 dark:text-gray-400 mt-1">
              Create, view, and delete permission tuples
            </p>
          </div>
          <button onClick={() => setShowForm(!showForm)} className="btn btn-primary">
            {showForm ? 'Cancel' : '+ New Tuple'}
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <div>
            <label className="block text-sm font-medium mb-2">Search</label>
            <input
              type="text"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="input"
              placeholder="Search tuples..."
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Filter by Namespace</label>
            <select
              value={filterNamespace}
              onChange={(e) => setFilterNamespace(e.target.value)}
              className="input"
            >
              <option value="">All Namespaces</option>
              {namespaces.map((ns) => (
                <option key={ns} value={ns}>
                  {ns}
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {showForm && (
        <TupleForm
          onSuccess={() => {
            setShowForm(false)
            writeMutation.reset()
          }}
          onCancel={() => setShowForm(false)}
        />
      )}

      <TupleTable
        tuples={filteredTuples}
        onDelete={(tuple) => {
          if (confirm(`Delete tuple: ${tuple.subject_namespace}:${tuple.subject_id} → ${tuple.relation} → ${tuple.namespace}:${tuple.object_id}?`)) {
            deleteMutation.mutate(tuple)
          }
        }}
      />
    </div>
  )
}

