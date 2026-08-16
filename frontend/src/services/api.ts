import type {
  RelationTuple,
  CheckRequest,
  CheckResponse,
  BatchCheckRequest,
  BatchCheckResponse,
  WriteRequest,
  WriteResponse,
  ListSubjectsRequest,
  ListSubjectsResponse,
} from '../types/api'

// Use relative URLs so Vite proxy can forward to backend HTTP gateway
// The proxy in vite.config.ts forwards /api/* to http://localhost:8081/api/*
const API_URL = import.meta.env.VITE_API_URL || ''

// Helper to convert gRPC-style to REST API
// Note: This assumes a REST gateway is added to the backend
// For now, we'll create a mock that can be replaced with actual gRPC-Web client

class ZenithAPI {
  private baseUrl: string

  constructor(baseUrl: string = API_URL) {
    // Use empty string for relative URLs if no explicit baseUrl provided
    this.baseUrl = baseUrl || ''
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    // Use relative URL so Vite proxy can handle it
    const url = `${this.baseUrl}${endpoint}`
    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
    })

    if (!response.ok) {
      const error = await response.text()
      throw new Error(`API error: ${error}`)
    }

    return response.json()
  }

  async check(req: CheckRequest): Promise<CheckResponse> {
    const start = performance.now()
    try {
      const response = await this.request<CheckResponse>('/api/check', {
        method: 'POST',
        body: JSON.stringify(req),
      })
      const latency = performance.now() - start
      return { ...response, latency }
    } catch (error) {
      // Fallback: if REST endpoint doesn't exist, we'll need gRPC-Web
      console.warn('REST API not available, using mock:', error)
      // For now, return a mock response
      return {
        allowed: false,
        zookie: '0',
        latency: performance.now() - start,
        cache_hit: false,
        expansion_depth: 0,
      }
    }
  }

  async batchCheck(req: BatchCheckRequest): Promise<BatchCheckResponse> {
    const start = performance.now()
    try {
      const response = await this.request<BatchCheckResponse>('/api/batch-check', {
        method: 'POST',
        body: JSON.stringify(req),
      })
      const totalLatency = performance.now() - start
      return { ...response, total_latency: totalLatency }
    } catch (error) {
      console.warn('REST API not available, using mock:', error)
      return {
        results: req.requests.map(() => ({ allowed: false, zookie: '0' })),
        zookie: '0',
        total_latency: performance.now() - start,
      }
    }
  }

  async write(req: WriteRequest): Promise<WriteResponse> {
    return this.request<WriteResponse>('/api/tuples', {
      method: req.operation === 'insert' ? 'POST' : 'DELETE',
      body: JSON.stringify(req.tuple),
    })
  }

  async listTuples(): Promise<RelationTuple[]> {
    try {
      return this.request<RelationTuple[]>('/api/tuples', {
        method: 'GET',
      })
    } catch (error) {
      console.warn('REST API not available, using mock:', error)
      return []
    }
  }

  async listSubjects(req: ListSubjectsRequest): Promise<ListSubjectsResponse> {
    try {
      return this.request<ListSubjectsResponse>(
        `/api/subjects/${req.namespace}/${req.object_id}/${req.relation}`,
        {
          method: 'GET',
        }
      )
    } catch (error) {
      console.warn('REST API not available, using mock:', error)
      return {
        subjects: [],
        zookie: '0',
      }
    }
  }
}

export const api = new ZenithAPI()
export default api

