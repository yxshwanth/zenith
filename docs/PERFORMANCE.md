# Zenith Performance Guide

## Performance Characteristics

Zenith is designed for sub-10ms latency for direct permission checks and <50ms for nested userset expansion (2-3 levels).

### Typical Latencies

- **Direct Check**: <5ms (cache hit) to <10ms (cache miss)
- **2-Level Expansion**: 10-20ms
- **3-Level Expansion**: 20-40ms
- **Deep Nesting (5+ levels)**: 40-100ms (may timeout)

### Throughput

- **With Caching**: 5,000-10,000 requests/second per instance
- **Without Caching**: 1,000-2,000 requests/second per instance
- **Write Operations**: 500-1,000 operations/second

## Benchmark Results

### Check Operations

```
BenchmarkCheck_Direct-8             50000    25000 ns/op    5000 B/op    50 allocs/op
BenchmarkCheck_Nested-8              20000    60000 ns/op   12000 B/op   120 allocs/op
```

### Expansion Engine

```
BenchmarkExpansion_Direct-8          50000    24000 ns/op    4800 B/op    48 allocs/op
BenchmarkExpansion_2Level-8          30000    45000 ns/op    9000 B/op    90 allocs/op
BenchmarkExpansion_3Level-8         20000    75000 ns/op   15000 B/op   150 allocs/op
```

### Cache Operations

```
BenchmarkCache_GetSet-8             200000     8000 ns/op    1600 B/op    16 allocs/op
BenchmarkCache_GetSetWithPatterns-8 150000    10000 ns/op    2000 B/op    20 allocs/op
BenchmarkCache_InvalidateRelated-8   50000    25000 ns/op    5000 B/op    50 allocs/op
```

*Note: Benchmark results vary based on hardware and load. Run your own benchmarks for accurate numbers.*

## Performance Optimization Tips

### 1. Enable Caching

Caching provides the biggest performance improvement:

```yaml
cache:
  enabled: true
  size: 50000  # Increase for better hit rates
  ttl_positive: 30s
  ttl_negative: 5s
```

**Expected Impact**: 5-10x improvement in throughput for repeated checks

### 2. Tune Cache Size

- **Small deployments**: 10,000-50,000 entries
- **Medium deployments**: 50,000-200,000 entries
- **Large deployments**: 200,000+ entries

Monitor cache hit rates via metrics:
```bash
curl http://localhost:9090/metrics | grep cache
```

### 3. Optimize Expansion Depth

Reduce max depth if experiencing timeouts:

```yaml
engine:
  max_depth: 5  # Reduce from default 10 if needed
  check_timeout_ms: 20  # Increase timeout if needed
```

**Trade-off**: Lower depth = faster but may miss deep permissions

### 4. Database Connection Pool

Tune based on database capacity:

```yaml
database:
  max_open_conns: 50  # Increase for higher load
  max_idle_conns: 10
  conn_max_lifetime: 5m
```

**Rule of thumb**: `max_open_conns` = (expected QPS / database QPS capacity) * 2

### 5. Rate Limiting

Configure rate limits to prevent overload:

```yaml
rate_limit:
  enabled: true
  global_rps: 5000
  per_client_rps: 500
  burst_size: 10
```

**Impact**: Protects system but may reject legitimate traffic if too restrictive

### 6. Reduce Expansion Depth

If you have control over your permission model:
- Flatten deeply nested usersets
- Use direct permissions where possible
- Limit nesting to 2-3 levels

## Profiling and Analysis

### Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem ./...

# Run specific benchmark
go test -bench=BenchmarkCheck_Direct -benchmem ./internal/service

# Run with CPU profiling
go test -cpuprofile=cpu.prof -bench=. ./internal/service
go tool pprof cpu.prof

# Run with memory profiling
go test -memprofile=mem.prof -bench=. ./internal/service
go tool pprof mem.prof
```

### Using pprof

```bash
# Start server with profiling
go run -cpuprofile=cpu.prof ./cmd/server

# In another terminal, generate load
# Then analyze:
go tool pprof cpu.prof

# Interactive commands:
# (pprof) top10
# (pprof) list Check
# (pprof) web
```

### Using the Profile Script

```bash
./scripts/profile.sh
```

## Monitoring Performance

### Key Metrics

1. **Request Latency** (`zenith_request_duration_seconds`)
   - P50, P95, P99 percentiles
   - Alert if P99 > 100ms

2. **Cache Hit Rate**
   - `zenith_cache_hits_total / (zenith_cache_hits_total + zenith_cache_misses_total)`
   - Target: >80% for production

3. **Expansion Depth** (`zenith_expansion_depth`)
   - Monitor distribution
   - Alert if frequently hitting max depth

4. **Database Query Latency** (`zenith_database_query_duration_seconds`)
   - Alert if P95 > 50ms

5. **Error Rate**
   - `rate(zenith_requests_total{status="error"}[5m])`
   - Alert if >1%

### Grafana Dashboard Queries

**Request Rate:**
```
rate(zenith_requests_total[5m])
```

**P95 Latency:**
```
histogram_quantile(0.95, rate(zenith_request_duration_seconds_bucket[5m]))
```

**Cache Hit Rate:**
```
rate(zenith_cache_hits_total[5m]) / (rate(zenith_cache_hits_total[5m]) + rate(zenith_cache_misses_total[5m]))
```

**Expansion Depth P95:**
```
histogram_quantile(0.95, rate(zenith_expansion_depth_bucket[5m]))
```

## Known Bottlenecks

1. **Deep Nesting**: Expansion depth >5 levels significantly increases latency
2. **Cache Misses**: High cache miss rate reduces throughput
3. **Database Queries**: Slow database queries affect all operations
4. **Concurrent Expansion**: Many concurrent expansions can increase memory usage

## Scaling Guidelines

### Vertical Scaling

- **CPU**: More CPU cores improve concurrent request handling
- **Memory**: Increase for larger cache sizes
- **Database**: Scale CockroachDB for higher QPS

### Horizontal Scaling

- **Stateless Service**: Zenith is stateless (except cache), scales horizontally
- **Load Balancer**: Use gRPC-aware load balancer
- **Database**: CockroachDB supports horizontal scaling
- **Cache**: Consider distributed cache (Redis) for shared cache across instances

### Scaling Strategy

1. **Start**: Single instance, default settings
2. **Scale Up**: Increase cache size, connection pool
3. **Scale Out**: Add more instances behind load balancer
4. **Scale Database**: Add CockroachDB nodes

## Performance Testing

### Load Testing with ghz

```bash
# Install ghz
go install github.com/bojand/ghz/cmd/ghz@latest

# Run load test
ghz --insecure \
  --proto internal/api/zenith.proto \
  --call zenith.v1.Zenith/Check \
  -d '{
    "subject_namespace": "user",
    "subject_id": "alice",
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer"
  }' \
  -c 10 \
  -n 10000 \
  localhost:50051
```

### Stress Testing

```bash
# Run with increasing load
for i in 1 5 10 20 50; do
  echo "Testing with $i concurrent connections"
  ghz --insecure \
    --proto internal/api/zenith.proto \
    --call zenith.v1.Zenith/Check \
    -d '{...}' \
    -c $i \
    -n 1000 \
    localhost:50051
done
```

## Optimization Checklist

- [ ] Enable caching with appropriate size
- [ ] Tune cache TTLs based on data freshness requirements
- [ ] Configure appropriate expansion depth and timeout
- [ ] Tune database connection pool
- [ ] Enable rate limiting with appropriate limits
- [ ] Monitor cache hit rates
- [ ] Monitor expansion depth distribution
- [ ] Profile hot paths
- [ ] Optimize based on profiling results
- [ ] Load test with expected traffic patterns

## Performance Targets

- **P50 Latency**: <10ms for direct checks
- **P95 Latency**: <50ms for 2-level expansion
- **P99 Latency**: <100ms for 3-level expansion
- **Cache Hit Rate**: >80%
- **Throughput**: >5,000 requests/second per instance (with cache)
- **Error Rate**: <0.1%

## Troubleshooting Performance Issues

### High Latency

1. Check cache hit rates (low hit rate = high latency)
2. Review expansion depth (deep nesting = high latency)
3. Check database query latency
4. Review connection pool utilization
5. Check for resource constraints (CPU, memory)

### Low Throughput

1. Increase cache size
2. Enable rate limiting (may be too restrictive)
3. Scale horizontally (add more instances)
4. Optimize database queries
5. Check for bottlenecks in profiling

### High Memory Usage

1. Reduce cache size
2. Reduce max expansion depth
3. Check for memory leaks (use pprof)
4. Increase instance memory

### Timeouts

1. Increase `check_timeout_ms`
2. Reduce `max_depth`
3. Optimize permission model (flatten nesting)
4. Check database performance

