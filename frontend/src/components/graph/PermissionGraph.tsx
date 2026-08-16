import { useState, useCallback } from 'react'
import { useTuples } from '../../hooks/useZenithAPI'
import { usePermissionGraph } from '../../hooks/usePermissionGraph'
import GraphVisualization from './GraphVisualization'
import GraphControls from './GraphControls'

export default function PermissionGraph() {
  const { data: tuples, isLoading, error } = useTuples()
  const { nodes, links, hasCycles } = usePermissionGraph(tuples || [])
  const [selectedNode, setSelectedNode] = useState<string | null>(null)
  const [hoveredLink, setHoveredLink] = useState<number | null>(null)

  const handleNodeClick = useCallback((nodeId: string) => {
    setSelectedNode(nodeId === selectedNode ? null : nodeId)
  }, [selectedNode])

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
        <h1 className="text-2xl font-bold mb-4">Permission Graph Visualizer</h1>
        <p className="text-gray-600 dark:text-gray-400 mb-4">
          Interactive visualization of permission relationships. Drag nodes to rearrange,
          hover over edges to see relation details, and click nodes to view permissions.
        </p>
        {hasCycles && (
          <div className="mb-4 p-3 bg-yellow-100 dark:bg-yellow-900 rounded-lg">
            <p className="text-yellow-800 dark:text-yellow-200">
              ⚠️ Cycles detected in permission graph (highlighted in red)
            </p>
          </div>
        )}
        <GraphControls
          nodeCount={nodes.length}
          linkCount={links.length}
          hasCycles={hasCycles}
        />
      </div>

      <div className="card p-0 overflow-hidden">
        <GraphVisualization
          nodes={nodes}
          links={links}
          selectedNode={selectedNode}
          hoveredLink={hoveredLink}
          onNodeClick={handleNodeClick}
          onLinkHover={setHoveredLink}
        />
      </div>

      {selectedNode && (
        <div className="card">
          <h2 className="text-lg font-semibold mb-2">Node Details</h2>
          <div className="space-y-2">
            <p>
              <span className="font-medium">ID:</span> {selectedNode}
            </p>
            <p>
              <span className="font-medium">Type:</span>{' '}
              {nodes.find((n) => n.id === selectedNode)?.type}
            </p>
            <div>
              <span className="font-medium">Connected Links:</span>
              <ul className="list-disc list-inside mt-1">
                {links
                  .filter(
                    (link) =>
                      String(link.source) === selectedNode ||
                      String(link.target) === selectedNode
                  )
                  .map((link, idx) => (
                    <li key={idx}>
                      {String(link.source) === selectedNode
                        ? `→ ${String(link.target)} (${link.relation})`
                        : `${String(link.source)} (${link.relation}) →`}
                    </li>
                  ))}
              </ul>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

