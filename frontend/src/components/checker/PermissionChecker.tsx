import { useState, useMemo } from 'react'
import { useCheck, useTuples } from '../../hooks/useZenithAPI'
import type { CheckRequest } from '../../types/api'
import ExpansionPath from './ExpansionPath'
import CheckHistory from './CheckHistory'

export default function PermissionChecker() {
  const { data: tuples } = useTuples()
  const [subject, setSubject] = useState('user:alice')
  const [relation, setRelation] = useState('viewer')
  const [object, setObject] = useState('doc:doc_1')
  const [requiredZookie, setRequiredZookie] = useState('')

  // Extract available options from tuples
  const { subjects, objects, relations } = useMemo(() => {
    const subjectSet = new Set<string>()
    const objectSet = new Set<string>()
    const relationSet = new Set<string>()

    tuples?.forEach((tuple) => {
      subjectSet.add(`${tuple.subject_namespace}:${tuple.subject_id}`)
      objectSet.add(`${tuple.namespace}:${tuple.object_id}`)
      relationSet.add(tuple.relation)
    })

    return {
      subjects: Array.from(subjectSet).sort(),
      objects: Array.from(objectSet).sort(),
      relations: Array.from(relationSet).sort(),
    }
  }, [tuples])

  const [checkRequest, setCheckRequest] = useState<CheckRequest | null>(null)
  const { data: checkResult, isLoading, isFetching } = useCheck(
    checkRequest!,
    !!checkRequest
  )

  const handleCheck = () => {
    const [subjectNamespace, subjectId] = subject.split(':')
    const [objectNamespace, objectId] = object.split(':')

    if (!subjectNamespace || !subjectId || !objectNamespace || !objectId) {
      alert('Invalid format. Use format: namespace:id')
      return
    }

    setCheckRequest({
      subject_namespace: subjectNamespace,
      subject_id: subjectId,
      namespace: objectNamespace,
      object_id: objectId,
      relation,
      required_zookie: requiredZookie || undefined,
    })
  }

  return (
    <div className="space-y-4">
      <div className="card">
        <h1 className="text-2xl font-bold mb-4">Permission Checker</h1>
        <p className="text-gray-600 dark:text-gray-400 mb-6">
          Check if a subject has a specific relation on an object. See the expansion
          path showing how access was granted.
        </p>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <div>
            <label className="block text-sm font-medium mb-2">Subject</label>
            <input
              type="text"
              list="subjects"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              className="input"
              placeholder="user:alice"
            />
            <datalist id="subjects">
              {subjects.map((s) => (
                <option key={s} value={s} />
              ))}
            </datalist>
          </div>

          <div>
            <label className="block text-sm font-medium mb-2">Relation</label>
            <select
              value={relation}
              onChange={(e) => setRelation(e.target.value)}
              className="input"
            >
              {relations.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium mb-2">Object</label>
            <input
              type="text"
              list="objects"
              value={object}
              onChange={(e) => setObject(e.target.value)}
              className="input"
              placeholder="doc:doc_1"
            />
            <datalist id="objects">
              {objects.map((o) => (
                <option key={o} value={o} />
              ))}
            </datalist>
          </div>

          <div>
            <label className="block text-sm font-medium mb-2">
              Required Zookie (optional)
            </label>
            <input
              type="text"
              value={requiredZookie}
              onChange={(e) => setRequiredZookie(e.target.value)}
              className="input"
              placeholder="For consistency demo"
            />
          </div>
        </div>

        <button
          onClick={handleCheck}
          disabled={isLoading || isFetching}
          className="btn btn-primary"
        >
          {isLoading || isFetching ? 'Checking...' : 'Check Permission'}
        </button>
      </div>

      {checkResult && (
        <>
          <div className="card">
            <div
              className={`p-4 rounded-lg ${
                checkResult.allowed
                  ? 'bg-green-100 dark:bg-green-900'
                  : 'bg-red-100 dark:bg-red-900'
              }`}
            >
              <div className="flex items-center space-x-2">
                <span className="text-2xl">
                  {checkResult.allowed ? '✅' : '❌'}
                </span>
                <span className="text-xl font-bold">
                  {checkResult.allowed ? 'ALLOWED' : 'DENIED'}
                </span>
              </div>
            </div>

            <div className="mt-4 space-y-2">
              <div className="flex justify-between">
                <span className="font-medium">Latency:</span>
                <span>{checkResult.latency?.toFixed(2)}ms</span>
              </div>
              <div className="flex justify-between">
                <span className="font-medium">Cache:</span>
                <span>{checkResult.cache_hit ? '✓ Hit' : '✗ Miss'}</span>
              </div>
              {checkResult.expansion_depth !== undefined && (
                <div className="flex justify-between">
                  <span className="font-medium">Expansion Depth:</span>
                  <span>{checkResult.expansion_depth}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="font-medium">Zookie:</span>
                <span className="font-mono text-sm">{checkResult.zookie}</span>
              </div>
            </div>
          </div>

          <ExpansionPath
            subject={subject}
            relation={relation}
            object={object}
            allowed={checkResult.allowed}
          />
        </>
      )}

      <CheckHistory />
    </div>
  )
}

