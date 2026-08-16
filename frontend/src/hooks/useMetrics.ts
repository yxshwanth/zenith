import { useQuery } from '@tanstack/react-query'
import { fetchMetrics } from '../services/metricsService'

export function useMetrics(refetchInterval: number = 5000) {
  return useQuery({
    queryKey: ['metrics'],
    queryFn: fetchMetrics,
    refetchInterval,
  })
}

