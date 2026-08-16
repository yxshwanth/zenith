interface GraphControlsProps {
  nodeCount: number
  linkCount: number
  hasCycles: boolean
}

export default function GraphControls({
  nodeCount,
  linkCount,
  hasCycles,
}: GraphControlsProps) {
  return (
    <div className="flex flex-wrap gap-4 text-sm">
      <div className="flex items-center space-x-2">
        <span className="font-medium">Nodes:</span>
        <span className="px-2 py-1 bg-blue-100 dark:bg-blue-900 rounded">
          {nodeCount}
        </span>
      </div>
      <div className="flex items-center space-x-2">
        <span className="font-medium">Links:</span>
        <span className="px-2 py-1 bg-green-100 dark:bg-green-900 rounded">
          {linkCount}
        </span>
      </div>
      {hasCycles && (
        <div className="flex items-center space-x-2">
          <span className="font-medium">Status:</span>
          <span className="px-2 py-1 bg-red-100 dark:bg-red-900 rounded text-red-800 dark:text-red-200">
            Cycles Detected
          </span>
        </div>
      )}
    </div>
  )
}

