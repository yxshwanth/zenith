import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '../services/api'
import type {
  CheckRequest,
  CheckResponse,
  BatchCheckRequest,
  BatchCheckResponse,
  WriteRequest,
  WriteResponse,
  RelationTuple,
  ListSubjectsRequest,
  ListSubjectsResponse,
} from '../types/api'

// Query keys
export const queryKeys = {
  check: (req: CheckRequest) => ['check', req] as const,
  batchCheck: (req: BatchCheckRequest) => ['batch-check', req] as const,
  tuples: () => ['tuples'] as const,
  subjects: (req: ListSubjectsRequest) => ['subjects', req] as const,
}

// Check permission hook
export function useCheck(req: CheckRequest, enabled: boolean = true) {
  return useQuery<CheckResponse>({
    queryKey: queryKeys.check(req),
    queryFn: () => api.check(req),
    enabled,
    staleTime: 5000, // 5 seconds
  })
}

// Batch check hook
export function useBatchCheck(req: BatchCheckRequest, enabled: boolean = true) {
  return useQuery<BatchCheckResponse>({
    queryKey: queryKeys.batchCheck(req),
    queryFn: () => api.batchCheck(req),
    enabled,
  })
}

// List tuples hook
export function useTuples() {
  return useQuery<RelationTuple[]>({
    queryKey: queryKeys.tuples(),
    queryFn: () => api.listTuples(),
    refetchInterval: 5000, // Refetch every 5 seconds
  })
}

// List subjects hook
export function useListSubjects(req: ListSubjectsRequest, enabled: boolean = true) {
  return useQuery<ListSubjectsResponse>({
    queryKey: queryKeys.subjects(req),
    queryFn: () => api.listSubjects(req),
    enabled,
  })
}

// Write tuple mutation
export function useWriteTuple() {
  const queryClient = useQueryClient()

  return useMutation<WriteResponse, Error, WriteRequest>({
    mutationFn: (req) => api.write(req),
    onSuccess: () => {
      // Invalidate tuples query to refetch
      queryClient.invalidateQueries({ queryKey: queryKeys.tuples() })
    },
  })
}

// Delete tuple mutation
export function useDeleteTuple() {
  const queryClient = useQueryClient()

  return useMutation<WriteResponse, Error, RelationTuple>({
    mutationFn: (tuple) =>
      api.write({
        tuple,
        operation: 'delete',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.tuples() })
    },
  })
}

