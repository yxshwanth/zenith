interface ExpansionPathProps {
  subject: string
  relation: string
  object: string
  allowed: boolean
}

export default function ExpansionPath({
  subject,
  relation,
  object,
  allowed,
}: ExpansionPathProps) {
  if (!allowed) {
    return (
      <div className="card">
        <h2 className="text-lg font-semibold mb-2">Expansion Path</h2>
        <p className="text-gray-600 dark:text-gray-400">
          No path found. Permission denied.
        </p>
      </div>
    )
  }

  // Simplified expansion path visualization
  // In a real implementation, this would show the actual path from the API response
  return (
    <div className="card">
      <h2 className="text-lg font-semibold mb-4">Expansion Path</h2>
      <div className="space-y-2 font-mono text-sm">
        <div>{subject}</div>
        <div className="ml-4">└─ member ──&gt; group:eng</div>
        <div className="ml-8">└─ {relation} ──&gt; {object}</div>
      </div>
      <p className="mt-4 text-xs text-gray-500 dark:text-gray-400">
        Note: This is a simplified visualization. The actual expansion path would be
        provided by the backend API.
      </p>
    </div>
  )
}

