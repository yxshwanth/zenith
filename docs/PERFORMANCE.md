# Zenith Performance Guide

Numbers below were measured with `go run ./cmd/bench` on 2026-08-16.

**Machine:** Apple M3, 16 GB RAM, darwin/arm64, Go 1.25.5  
**Database:** CockroachDB single-node (Docker), loopback  
**Dataset:** 75,002 relation tuples — 50k direct docs, 1,000 groups × 20 members, 5k nested docs  
**Load:** 20,000 gRPC Checks × 50 clients, 2,000 warmup, `check_timeout_ms=50`, tracing off

Reproduce: `./scripts/bench.sh` (server must be listening on `:50051`).

## What changed

1. **One SQL round trip per direct lookup.** `CheckDirect` used to `SELECT 1` and then `SELECT cluster_logical_timestamp()`. It now returns existence and the zookie together.
2. **Single-userset expansion stays on the calling goroutine.** Fan-out is only for two or more userset branches.
3. **Cache stores the finished Check.** A direct miss is not written to the LRU before userset expansion; doing so cached nested allows as denies.

## SQL microbench (no gRPC)

Same tuple, same connection pool, 3,000 iterations after 200 warmup.

| Path | p50 | p95 | p99 | avg | QPS |
| --- | --- | --- | --- | --- | --- |
| Legacy 2-RTT (`SELECT 1` + `GetZookie`) | 297 µs | 454 µs | 802 µs | 322 µs | 3,103 |
| Combined 1-RTT (`EXISTS` + zookie) | 194 µs | 453 µs | 1.00 ms | 240 µs | 4,159 |

p50 −35%. Throughput +34%. This is loopback; a 1 ms database RTT would make the two-query path ~2× the one-query path.

## gRPC Check

### Cache on

| Scenario | p50 | p95 | p99 | avg | QPS | Allows |
| --- | --- | --- | --- | --- | --- | --- |
| Direct, hot key | 387 µs | 807 µs | 996 µs | 437 µs | 114,116 | 20,000 / 20,000 |
| Nested hot (doc → group#member → user) | 434 µs | 1.10 ms | 1.59 ms | 514 µs | 97,050 | 20,000 / 20,000 |
| Deny, hot key | 374 µs | 814 µs | 1.13 ms | 426 µs | 117,222 | 0 / 20,000 |
| Direct, uniform over 50k docs | 2.89 ms | 7.47 ms | 11.5 ms | 3.17 ms | 15,770 | allow |

### Cache off

| Scenario | p50 | p95 | p99 | avg | QPS | Allows |
| --- | --- | --- | --- | --- | --- | --- |
| Direct, hot key | 638 µs | 1.25 ms | 1.73 ms | 706 µs | 70,685 | 20,000 / 20,000 |
| Nested hot | 1.04 ms | 1.54 ms | 2.11 ms | 1.10 ms | 45,373 | 20,000 / 20,000 |
| Deny, hot key | 812 µs | 1.35 ms | 1.71 ms | 879 µs | 56,815 | 0 / 20,000 |
| Direct, uniform over 50k docs | 2.85 ms | 6.57 ms | 11.5 ms | 3.39 ms | 14,741 | allow |

Nested hot p50 is 2.4× lower with the cache on (434 µs vs 1.04 ms) because the cached value is the expansion result.

## In-process Go benchmarks

These hit an in-memory mock repo. They are useful for allocator regressions, not for latency SLOs.

```
go test -bench=. -benchmem ./internal/engine ./internal/cache ./internal/service
```

## How to run

```bash
docker compose up -d
docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'
go build -o zenith ./cmd/server
./zenith -config=config.yaml -check-timeout=50 -enable-tracing=false
```

```bash
go run ./cmd/bench -mode=seed
go run ./cmd/bench -mode=sql
go run ./cmd/bench -mode=load -n=20000 -c=50
```

Flags: `-direct-docs`, `-groups`, `-members`, `-nested-docs`, `-scenario=direct_hot|direct_uniform|nested_hot|deny|all`.

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

Measured on loopback M3 (see tables above). Treat these as the bar for a regression:

- **Hot direct Check, cache on:** p50 < 1 ms, p99 < 2 ms
- **Hot nested Check, cache on:** p50 < 1 ms, p99 < 3 ms
- **Uniform direct over a large keyspace:** p50 < 5 ms, p99 < 15 ms
- **Hot nested vs cache off:** cache-on p50 should stay ~2× better
- **SQL CheckDirect:** combined 1-RTT p50 should beat legacy 2-RTT

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

