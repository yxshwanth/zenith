// TypeScript types matching the gRPC proto definitions

export interface RelationTuple {
  namespace: string
  object_id: string
  relation: string
  subject_namespace: string
  subject_id: string
  subject_relation: string
}

export interface CheckRequest {
  subject_namespace: string
  subject_id: string
  subject_relation?: string
  namespace: string
  object_id: string
  relation: string
  required_zookie?: string
}

export interface CheckResponse {
  allowed: boolean
  zookie: string
  latency?: number
  cache_hit?: boolean
  expansion_depth?: number
}

export interface CheckResult {
  allowed: boolean
  zookie: string
}

export interface BatchCheckRequest {
  requests: CheckRequest[]
  required_zookie?: string
}

export interface BatchCheckResponse {
  results: CheckResult[]
  zookie: string
  total_latency?: number
}

export interface WriteRequest {
  tuple: RelationTuple
  operation: 'insert' | 'delete'
}

export interface WriteResponse {
  zookie: string
}

export interface Subject {
  namespace: string
  id: string
  relation: string
}

export interface ListSubjectsRequest {
  namespace: string
  object_id: string
  relation: string
  required_zookie?: string
}

export interface ListSubjectsResponse {
  subjects: Subject[]
  zookie: string
}

