import { useMemo } from 'react'
import type { RelationTuple } from '../types/api'
import type { GraphNode, GraphLink } from '../types/graph'

export function usePermissionGraph(tuples: RelationTuple[]) {
  return useMemo(() => {
    const nodesMap = new Map<string, GraphNode>()
    const links: GraphLink[] = []

    // Process tuples to create nodes and links
    tuples.forEach((tuple) => {
      // Create subject node
      const subjectKey = `${tuple.subject_namespace}:${tuple.subject_id}`
      if (!nodesMap.has(subjectKey)) {
        nodesMap.set(subjectKey, {
          id: subjectKey,
          label: tuple.subject_id,
          type: tuple.subject_namespace,
        })
      }

      // Create object node
      const objectKey = `${tuple.namespace}:${tuple.object_id}`
      if (!nodesMap.has(objectKey)) {
        nodesMap.set(objectKey, {
          id: objectKey,
          label: tuple.object_id,
          type: tuple.namespace,
        })
      }

      // Create link
      links.push({
        source: subjectKey,
        target: objectKey,
        relation: tuple.relation,
        namespace: tuple.namespace,
      })
    })

    const nodes = Array.from(nodesMap.values())

    // Detect cycles (simplified DFS)
    const cycles = detectCycles(nodes, links)
    const cycleLinks = new Set(cycles.map((c) => `${c.source}-${c.target}`))

    return {
      nodes,
      links: links.map((link) => ({
        ...link,
        isCycle: cycleLinks.has(`${link.source}-${link.target}`),
      })),
      hasCycles: cycles.length > 0,
    }
  }, [tuples])
}

function detectCycles(nodes: GraphNode[], links: GraphLink[]): GraphLink[] {
  const visited = new Set<string>()
  const recStack = new Set<string>()
  const cycleLinks: GraphLink[] = []

  const dfs = (nodeId: string, parentId: string | null): boolean => {
    visited.add(nodeId)
    recStack.add(nodeId)

    const outgoingLinks = links.filter(
      (link) => String(link.source) === nodeId
    )

    for (const link of outgoingLinks) {
      const targetId = String(link.target)
      if (!visited.has(targetId)) {
        if (dfs(targetId, nodeId)) {
          cycleLinks.push(link)
          return true
        }
      } else if (recStack.has(targetId) && targetId !== parentId) {
        // Cycle detected
        cycleLinks.push(link)
        return true
      }
    }

    recStack.delete(nodeId)
    return false
  }

  for (const node of nodes) {
    if (!visited.has(node.id)) {
      dfs(node.id, null)
    }
  }

  return cycleLinks
}

