import { useEffect, useRef, useState } from 'react'
import { Group } from '@visx/group'
import { Line } from '@visx/shape'
import { Text } from '@visx/text'
import type { GraphNode, GraphLink } from '../../types/graph'

interface GraphVisualizationProps {
  nodes: GraphNode[]
  links: GraphLink[]
  selectedNode: string | null
  hoveredLink: number | null
  onNodeClick: (nodeId: string) => void
  onLinkHover: (linkIndex: number | null) => void
}

const width = 1200
const height = 800
const nodeRadius = 20

const nodeColors: Record<string, string> = {
  user: '#3b82f6', // blue
  group: '#10b981', // green
  doc: '#f59e0b', // orange
  folder: '#8b5cf6', // purple
  default: '#6b7280', // gray
}

export default function GraphVisualization({
  nodes,
  links,
  selectedNode,
  hoveredLink,
  onNodeClick,
  onLinkHover,
}: GraphVisualizationProps) {
  const svgRef = useRef<SVGSVGElement>(null)
  const [positions, setPositions] = useState<Map<string, { x: number; y: number }>>(
    new Map()
  )
  const [draggedNode, setDraggedNode] = useState<string | null>(null)

  // Initialize positions with force-directed layout
  useEffect(() => {
    if (nodes.length === 0) return

    const newPositions = new Map<string, { x: number; y: number }>()
    const centerX = width / 2
    const centerY = height / 2
    const radius = Math.min(width, height) / 3

    // Simple circular layout for initial positions
    nodes.forEach((node, index) => {
      const angle = (2 * Math.PI * index) / nodes.length
      newPositions.set(node.id, {
        x: centerX + radius * Math.cos(angle),
        y: centerY + radius * Math.sin(angle),
      })
    })

    setPositions(newPositions)
  }, [nodes])

  const handleMouseDown = (nodeId: string, e: React.MouseEvent) => {
    e.stopPropagation()
    setDraggedNode(nodeId)
  }

  const handleMouseMove = (e: React.MouseEvent) => {
    if (!draggedNode || !svgRef.current) return

    const rect = svgRef.current.getBoundingClientRect()
    const x = e.clientX - rect.left
    const y = e.clientY - rect.top

    setPositions((prev) => {
      const next = new Map(prev)
      next.set(draggedNode, { x, y })
      return next
    })
  }

  const handleMouseUp = () => {
    setDraggedNode(null)
  }

  if (nodes.length === 0) {
    return (
      <div className="flex items-center justify-center h-96 text-gray-500 dark:text-gray-400">
        No tuples to visualize. Add some tuples to see the graph.
      </div>
    )
  }

  return (
    <div className="relative">
      <svg
        ref={svgRef}
        width={width}
        height={height}
        className="border border-gray-200 dark:border-gray-700"
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onMouseLeave={handleMouseUp}
      >
        <Group>
          {/* Render links */}
          {links.map((link, index) => {
            const sourceId = String(link.source)
            const targetId = String(link.target)
            const sourcePos = positions.get(sourceId)
            const targetPos = positions.get(targetId)

            if (!sourcePos || !targetPos) return null

            const isHovered = hoveredLink === index
            const isCycle = (link as any).isCycle

            return (
              <g key={index}>
                <Line
                  from={{ x: sourcePos.x, y: sourcePos.y }}
                  to={{ x: targetPos.x, y: targetPos.y }}
                  stroke={isCycle ? '#ef4444' : isHovered ? '#3b82f6' : '#9ca3af'}
                  strokeWidth={isHovered ? 3 : isCycle ? 2 : 1}
                  strokeDasharray={isCycle ? '5,5' : undefined}
                  onMouseEnter={() => onLinkHover(index)}
                  onMouseLeave={() => onLinkHover(null)}
                  style={{ cursor: 'pointer' }}
                />
                {/* Relation label */}
                <Text
                  x={(sourcePos.x + targetPos.x) / 2}
                  y={(sourcePos.y + targetPos.y) / 2}
                  fontSize={12}
                  fill={isHovered ? '#3b82f6' : '#6b7280'}
                  textAnchor="middle"
                  pointerEvents="none"
                >
                  {link.relation}
                </Text>
              </g>
            )
          })}

          {/* Render nodes */}
          {nodes.map((node) => {
            const pos = positions.get(node.id)
            if (!pos) return null

            const isSelected = selectedNode === node.id
            const color = nodeColors[node.type] || nodeColors.default

            return (
              <g
                key={node.id}
                transform={`translate(${pos.x}, ${pos.y})`}
                onMouseDown={(e) => handleMouseDown(node.id, e)}
                onClick={() => onNodeClick(node.id)}
                style={{ cursor: 'pointer' }}
              >
                <circle
                  r={nodeRadius}
                  fill={isSelected ? color : color}
                  fillOpacity={isSelected ? 1 : 0.7}
                  stroke={isSelected ? '#1e40af' : 'none'}
                  strokeWidth={isSelected ? 3 : 0}
                />
                <Text
                  y={nodeRadius + 15}
                  fontSize={12}
                  fill="#374151"
                  textAnchor="middle"
                  pointerEvents="none"
                >
                  {node.label}
                </Text>
                <Text
                  y={nodeRadius + 28}
                  fontSize={10}
                  fill="#6b7280"
                  textAnchor="middle"
                  pointerEvents="none"
                >
                  {node.type}
                </Text>
              </g>
            )
          })}
        </Group>
      </svg>
    </div>
  )
}

