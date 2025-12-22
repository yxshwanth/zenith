# Zenith Development Guide

## Architecture Overview

Zenith is a permission system built with Go, using CockroachDB for storage and gRPC for the API. The system implements a relation tuple model with recursive userset expansion.

### High-Level Architecture

```
┌─────────────┐
│   gRPC API  │
│  (Service)  │
└──────┬──────┘
       │
       ├──► Expansion Engine (Forward)
       ├──► Reverse Expansion Engine
       ├──► Cache Layer
       └──► Database Layer (CockroachDB)
```

### Component Overview

1. **Service Layer** (`internal/service/`)
   - gRPC service implementation
   - Request validation
   - Singleflight deduplication
   - Error handling

2. **Engine Layer** (`internal/engine/`)
   - Forward expansion (Check operations)
   - Reverse expansion (ListSubjects operations)
   - Cycle detection
   - Timeout handling

3. **Database Layer** (`internal/db/`)
   - Connection management
   - Tuple repository
   - Zookie handling
   - Query execution

4. **Cache Layer** (`internal/cache/`)
   - LRU cache with TTL
   - Negative caching
   - Sub-graph caching
   - Selective invalidation

5. **Configuration** (`internal/config/`)
   - YAML/JSON file support
   - Environment variable support
   - Flag overrides

6. **Middleware** (`internal/middleware/`)
   - Rate limiting
   - Future: Authentication, logging

7. **Observability** (`internal/observability/`)
   - OpenTelemetry tracing
   - Metrics (Prometheus)

## Code Organization

```
zenith/
├── cmd/
│   └── server/          # Application entry point
├── internal/
│   ├── api/            # gRPC service definitions (.proto)
│   ├── cache/          # Caching implementation
│   ├── config/         # Configuration management
│   ├── db/             # Database layer
│   ├── engine/         # Expansion engines
│   ├── errors/         # Custom error types
│   ├── metrics/        # Prometheus metrics
│   ├── middleware/     # gRPC middleware (rate limiting)
│   ├── models/         # Domain models
│   ├── observability/  # Tracing and observability
│   └── service/        # gRPC service implementation
├── docs/               # Documentation
├── scripts/            # Utility scripts
└── docker-compose.yml  # Local development setup
```

## Local Development Setup

### Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- Protocol Buffers compiler (`protoc`)
- CockroachDB (via Docker)

### Setup Steps

1. **Clone and Install Dependencies**
   ```bash
   git clone <repository>
   cd zenith
   go mod download
   ```

2. **Install Protobuf Tools**
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```

3. **Generate Protobuf Code**
   ```bash
   make proto
   # Or manually:
   protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       internal/api/zenith.proto
   ```

4. **Start CockroachDB**
   ```bash
   docker-compose up -d
   ```

5. **Create Database**
   ```bash
   docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'
   ```

6. **Build and Run**
   ```bash
   go build -o zenith ./cmd/server
   ./zenith
   ```

## Adding New Features

### 1. Adding a New gRPC Endpoint

1. **Define in Protobuf** (`internal/api/zenith.proto`)
   ```protobuf
   rpc NewEndpoint(NewEndpointRequest) returns (NewEndpointResponse);
   ```

2. **Generate Code**
   ```bash
   make proto
   ```

3. **Implement in Service** (`internal/service/zenith.go`)
   ```go
   func (s *Service) NewEndpoint(ctx context.Context, req *api.NewEndpointRequest) (*api.NewEndpointResponse, error) {
       // Implementation
   }
   ```

4. **Add Tests** (`internal/service/zenith_test.go`)

### 2. Adding a New Configuration Option

1. **Add to Config Struct** (`internal/config/config.go`)
   ```go
   type Config struct {
       // ... existing fields
       NewOption string `yaml:"new_option" json:"new_option"`
   }
   ```

2. **Set Default** (`DefaultConfig()`)

3. **Add Environment Variable Support** (`LoadFromEnv()`)

4. **Add Flag** (`cmd/server/main.go`)

5. **Use in Main** (`cmd/server/main.go`)

### 3. Adding Database Queries

1. **Add Method to TupleRepo** (`internal/db/tuple_repo.go`)
   ```go
   func (r *TupleRepo) NewQuery(ctx context.Context, ...) (..., error) {
       // Implementation with zookie support
   }
   ```

2. **Add Tests** (`internal/db/tuple_repo_test.go`)

## Testing Guidelines

### Unit Tests

- Place tests in `*_test.go` files in the same package
- Use table-driven tests for multiple scenarios
- Mock external dependencies (database, cache)

**Example:**
```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "test", "result", false},
        {"invalid input", "", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Feature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Feature() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Feature() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Integration Tests

- Use real database connections
- Clean up test data
- Test end-to-end scenarios

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package
go test ./internal/service

# Run with verbose output
go test -v ./...
```

## Debugging Tips

### 1. Enable Tracing

Start server with tracing enabled:
```bash
./zenith -enable-tracing=true -tracing-endpoint=localhost:4317
```

View traces in your OpenTelemetry backend (Jaeger, Zipkin, etc.).

### 2. Enable Verbose Logging

Add logging statements:
```go
log.Printf("Debug: %+v", variable)
```

### 3. Use gRPC Reflection

Reflection is enabled by default. Use tools like `grpcurl` or `grpcui`:
```bash
grpcurl -plaintext localhost:50051 list
```

### 4. Database Queries

Connect to CockroachDB:
```bash
docker exec -it zenith-cockroachdb ./cockroach sql --insecure
```

Query tuples:
```sql
SELECT * FROM relation_tuples LIMIT 10;
```

### 5. Metrics

View metrics:
```bash
curl http://localhost:9090/metrics
```

## Code Style

- Follow Go conventions (gofmt, golint)
- Use meaningful variable names
- Add comments for exported functions
- Keep functions focused and small
- Handle errors explicitly (don't ignore)

## Performance Considerations

- **Database Queries**: Use indexes, avoid N+1 queries
- **Caching**: Cache frequently accessed data
- **Concurrency**: Use goroutines for parallel operations
- **Memory**: Be mindful of cache sizes and expansion depth

## Common Patterns

### Error Handling

```go
import "github.com/zenith/zenith/internal/errors"

// Create error
err := errors.NewValidationError("invalid input", map[string]interface{}{
    "field": "namespace",
})

// Convert to gRPC status
return nil, errors.ToGRPCStatus(err)

// Add to span
errors.AddToSpan(span, err)
```

### Using Zookies

```go
// Get zookie after write
zookie, err := repo.Insert(ctx, tuple)

// Use in check
allowed, zookie, err := repo.CheckDirect(ctx, tuple, requiredZookie)
```

### Caching

```go
// Check cache
if entry, ok := cache.GetCheck(key); ok {
    return entry.Allowed, entry.Zookie, nil
}

// Set cache
cache.SetCheckWithPatterns(key, allowed, zookie, ...)
```

## Contribution Guidelines

1. **Fork and Branch**: Create a feature branch from `main`
2. **Write Tests**: Add tests for new features
3. **Update Documentation**: Update relevant docs
4. **Run Tests**: Ensure all tests pass
5. **Submit PR**: Create pull request with description

## Troubleshooting

### Build Errors

- **Missing protobuf files**: Run `make proto`
- **Import errors**: Run `go mod download`
- **Database connection**: Check CockroachDB is running

### Test Failures

- **Database connection**: Ensure test database is available
- **Port conflicts**: Change ports in config
- **Race conditions**: Run with `-race` flag

### Runtime Issues

- **High latency**: Check database connection, cache hit rates
- **Memory usage**: Adjust cache size
- **Connection errors**: Check database health

