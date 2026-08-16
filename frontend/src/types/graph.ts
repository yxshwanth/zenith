export interface GraphNode {
  id: string
  label: string
  type: string // 'user' | 'group' | 'doc' | 'folder' | etc.
  x?: number
  y?: number
  vx?: number
  vy?: number
}

export interface GraphLink {
  source: string | GraphNode
  target: string | GraphNode
  relation: string
  namespace?: string
  zookie?: string
}

export interface GraphData {
  nodes: GraphNode[]
  links: GraphLink[]
}

