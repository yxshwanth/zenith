# Testing Guide for Zenith

This guide covers all testing approaches for the Zenith permission system.

## Table of Contents

1. [Unit Tests](#unit-tests)
2. [Integration Tests](#integration-tests)
3. [Manual Testing](#manual-testing)
4. [Performance Testing](#performance-testing)
5. [Testing Phase 4 Features](#testing-phase-4-features)

## Prerequisites

Before testing, ensure you have:

1. **CockroachDB running**:
   ```bash
   docker-compose up -d
   ```

2. **Database created**:
   ```bash
   docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'
   ```

3. **grpcurl installed** (for integration tests):
   ```bash
   go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
   export PATH="$(go env GOPATH)/bin:$PATH"
   ```

## Unit Tests

Run all unit tests:

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test -v ./internal/cache
go test -v ./internal/engine
go test -v ./internal/db
go test -v ./internal/service

# Run tests with coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Packages

- **`internal/cache`**: Tests for negative caching, sub-graph caching, expiration, and invalidation
- **`internal/engine`**: Tests for recursive expansion, cycle detection, timeout handling, and zookie consistency
- **`internal/db`**: Tests for database connection and zookie handling
- **`internal/service`**: Tests for service layer (singleflight deduplication)

## Integration Tests

### Quick Test Script

The quick test script runs basic Write and Check operations:

```bash
# Start the server in one terminal
./zenith

# In another terminal, run the quick test
./scripts/quick-test.sh
```

### Comprehensive Test Script

The full test script includes more scenarios:

```bash
# Start the server
./zenith

# Run comprehensive tests
./scripts/test.sh

# Or test against a different server
./scripts/test.sh localhost:50051
```

### Manual Integration Testing

#### 1. Start the Server

```bash
# Build the server
go build -o zenith ./cmd/server

# Run with default settings
./zenith

# Or with custom configuration
./zenith \
  -port=50051 \
  -db="postgres://root@localhost:26257/zenith?sslmode=disable" \
  -enable-cache=true \
  -cache-size=10000 \
  -cache-ttl-positive=30s \
  -cache-ttl-negative=5s \
  -enable-tracing=true \
  -tracing-endpoint=localhost:4317 \
  -max-depth=10 \
  -check-timeout=10
```

#### 2. Test Write Operations

```bash
# Write a direct user tuple
grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "user",
    "subject_id": "alice"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' localhost:50051 zenith.v1.Zenith/Write

# Write a userset tuple
grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "group",
    "subject_id": "eng",
    "subject_relation": "member"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' localhost:50051 zenith.v1.Zenith/Write
```

#### 3. Test Check Operations

```bash
# Check direct user permission
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check

# Check with required_zookie (causal consistency)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer",
  "required_zookie": 1766433684599320881
}' localhost:50051 zenith.v1.Zenith/Check

# Check nested userset (recursive expansion)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "bob",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

#### 4. Test Recursive Expansion

Set up a nested scenario:

```bash
# 1. User is member of group
grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "group",
    "object_id": "eng",
    "relation": "member",
    "subject_namespace": "user",
    "subject_id": "bob"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' localhost:50051 zenith.v1.Zenith/Write

# 2. Group has access to document
grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "group",
    "subject_id": "eng",
    "subject_relation": "member"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' localhost:50051 zenith.v1.Zenith/Write

# 3. Check if user has access (should recursively expand)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "bob",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

## Performance Testing

### Benchmark Tests

```bash
# Run benchmarks
go test -bench=. ./internal/engine
go test -bench=. ./internal/cache

# Run with memory profiling
go test -bench=. -memprofile=mem.prof ./internal/cache
go tool pprof mem.prof
```

### Load Testing

Use a tool like `ghz` for gRPC load testing:

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
  -n 1000 \
  localhost:50051
```

### Cache Performance Testing

Test cache hit rates:

```bash
# Run server with cache enabled
./zenith -enable-cache=true -cache-size=10000

# Make repeated requests (should see cache hits)
for i in {1..100}; do
  grpcurl -plaintext -d '{
    "subject_namespace": "user",
    "subject_id": "alice",
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer"
  }' localhost:50051 zenith.v1.Zenith/Check
done
```

## Testing Phase 4 Features

### 1. Singleflight Deduplication

Test that concurrent identical requests are deduplicated:

```bash
# Run 10 concurrent identical requests
for i in {1..10}; do
  grpcurl -plaintext -d '{
    "subject_namespace": "user",
    "subject_id": "alice",
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer"
  }' localhost:50051 zenith.v1.Zenith/Check &
done
wait

# All should return the same result (check logs for single execution)
```

### 2. Cache Testing

Test negative caching:

```bash
# 1. Check a non-existent permission (should cache negative result)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "nonexistent",
  "namespace": "doc",
  "object_id": "doc_999",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check

# 2. Check again immediately (should hit cache)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "nonexistent",
  "namespace": "doc",
  "object_id": "doc_999",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

Test sub-graph caching:

```bash
# 1. Set up nested structure
# User -> Group -> Document

# 2. First check (should populate cache)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "bob",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check

# 3. Second check (should use cached sub-graph results)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "bob",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

### 3. Tracing Testing

Test OpenTelemetry tracing:

```bash
# Start server with tracing enabled
./zenith -enable-tracing=true -tracing-endpoint=localhost:4317

# Make a request
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check

# Check traces in your OTel collector/backend (Jaeger, Zipkin, etc.)
```

## Test Scenarios

### Scenario 1: Direct User Permission

```bash
# Write
grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "user",
    "subject_id": "alice"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' localhost:50051 zenith.v1.Zenith/Write

# Check
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

### Scenario 2: Nested Userset (2 levels)

```bash
# User is member of group
grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "group",
    "object_id": "eng",
    "relation": "member",
    "subject_namespace": "user",
    "subject_id": "bob"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' localhost:50051 zenith.v1.Zenith/Write

# Group has access to document
grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "group",
    "subject_id": "eng",
    "subject_relation": "member"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' localhost:50051 zenith.v1.Zenith/Write

# Check (should recursively expand)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "bob",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

### Scenario 3: Deep Nesting (3+ levels)

```bash
# User -> Subgroup -> Group -> Document
# (Similar pattern, just add more levels)
```

### Scenario 4: Causal Consistency (Zookie)

```bash
# 1. Write and get zookie
WRITE_RESPONSE=$(grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "user",
    "subject_id": "alice"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' localhost:50051 zenith.v1.Zenith/Write)

ZOOKIE=$(echo "$WRITE_RESPONSE" | grep -oE '"zookie":\s*"[0-9]+"' | grep -oE '[0-9]+' | head -1)

# 2. Check with required_zookie (ensures we see the write)
grpcurl -plaintext -d "{
  \"subject_namespace\": \"user\",
  \"subject_id\": \"alice\",
  \"namespace\": \"doc\",
  \"object_id\": \"doc_1\",
  \"relation\": \"viewer\",
  \"required_zookie\": $ZOOKIE
}" localhost:50051 zenith.v1.Zenith/Check
```

## Troubleshooting

### Tests Fail with "connection refused"

- Ensure CockroachDB is running: `docker-compose ps`
- Check database connection string matches your setup
- Verify database exists: `docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'SHOW DATABASES;'`

### Cache Not Working

- Verify cache is enabled: `./zenith -enable-cache=true`
- Check cache size is sufficient: `-cache-size=10000`
- Monitor cache stats (add logging if needed)

### Tracing Not Appearing

- Ensure OTel collector is running on the endpoint
- Check tracing is enabled: `./zenith -enable-tracing=true`
- Verify endpoint: `-tracing-endpoint=localhost:4317`

### Performance Issues

- Check database indexes are created
- Monitor cache hit rates
- Review expansion depth and timeout settings
- Use `go tool pprof` for profiling

## Continuous Integration

For CI/CD, you can run:

```bash
# Run all tests
go test ./...

# Run with coverage threshold
go test -cover -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total | awk '{if ($3+0 < 80) exit 1}'
```

## Next Steps

- Add more comprehensive integration tests
- Set up performance benchmarks
- Configure tracing backend (Jaeger, Zipkin, etc.)
- Add load testing to CI/CD pipeline

