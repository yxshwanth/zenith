import { useState } from 'react'
import { useWriteTuple } from '../../hooks/useZenithAPI'
import type { RelationTuple } from '../../types/api'

interface TupleFormProps {
  onSuccess: () => void
  onCancel: () => void
}

export default function TupleForm({ onSuccess, onCancel }: TupleFormProps) {
  const writeMutation = useWriteTuple()
  const [tuple, setTuple] = useState<RelationTuple>({
    namespace: 'doc',
    object_id: '',
    relation: 'viewer',
    subject_namespace: 'user',
    subject_id: '',
    subject_relation: '',
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()

    if (!tuple.object_id || !tuple.subject_id) {
      alert('Object ID and Subject ID are required')
      return
    }

    writeMutation.mutate(
      {
        tuple,
        operation: 'insert',
      },
      {
        onSuccess: (data) => {
          alert(`Tuple created! Zookie: ${data.zookie}`)
          onSuccess()
        },
        onError: (error) => {
          alert(`Error: ${error.message}`)
        },
      }
    )
  }

  return (
    <div className="card">
      <h2 className="text-lg font-semibold mb-4">Add New Tuple</h2>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium mb-2">Object Namespace</label>
            <input
              type="text"
              value={tuple.namespace}
              onChange={(e) => setTuple({ ...tuple, namespace: e.target.value })}
              className="input"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Object ID</label>
            <input
              type="text"
              value={tuple.object_id}
              onChange={(e) => setTuple({ ...tuple, object_id: e.target.value })}
              className="input"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Relation</label>
            <input
              type="text"
              value={tuple.relation}
              onChange={(e) => setTuple({ ...tuple, relation: e.target.value })}
              className="input"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Subject Namespace</label>
            <input
              type="text"
              value={tuple.subject_namespace}
              onChange={(e) =>
                setTuple({ ...tuple, subject_namespace: e.target.value })
              }
              className="input"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">Subject ID</label>
            <input
              type="text"
              value={tuple.subject_id}
              onChange={(e) => setTuple({ ...tuple, subject_id: e.target.value })}
              className="input"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium mb-2">
              Subject Relation (optional, for usersets)
            </label>
            <input
              type="text"
              value={tuple.subject_relation}
              onChange={(e) =>
                setTuple({ ...tuple, subject_relation: e.target.value })
              }
              className="input"
              placeholder="Leave empty for direct users"
            />
          </div>
        </div>

        <div className="flex space-x-2">
          <button
            type="submit"
            disabled={writeMutation.isPending}
            className="btn btn-primary"
          >
            {writeMutation.isPending ? 'Creating...' : 'Create Tuple'}
          </button>
          <button type="button" onClick={onCancel} className="btn btn-secondary">
            Cancel
          </button>
        </div>
      </form>
    </div>
  )
}

