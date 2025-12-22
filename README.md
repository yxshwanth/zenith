# Zenith - Unified Permission API

Zenith provides a unified API to answer: "Does subject S have relation R on object O?" The system handles deep nesting (e.g., user is in a group, which is in another group, which has access to a folder) while maintaining sub-10ms latency.

## Key Features

- **Unified Permission Model**: Relation tuples stored as a Directed Acyclic Graph (DAG)
- **Causal Consistency**: Zookie-based consistency using CockroachDB logical timestamps
- **High Performance**: Sub-10ms latency targets with optimized queries
- **gRPC API**: Type-safe, efficient protocol for permission operations

## Architecture

### Data Model

Every permission is stored as a relation tuple:
- **Namespace**: The type of object (e.g., "doc", "folder", "repository")
- **Object ID**: The unique identifier of the instance
- **Relation**: The role (e.g., "owner", "editor", "viewer")
- **Subject**: Either a direct user ID or a "userset" (another object#relation)

### Components

1. **gRPC API Layer**: Write and Check endpoints
2. **Database Layer**: CockroachDB with Zookie support
3. **Service Layer**: Business logic for tuple operations

## Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- Protocol Buffers compiler (`protoc`)
  - macOS: `brew install protobuf`
  - Linux: `apt-get install protobuf-compiler` or `yum install protobuf-compiler`
  - Or download from [Protocol Buffers releases](https://github.com/protocolbuffers/protobuf/releases)

## Setup

### 1. Install Dependencies

```bash
# Install Go dependencies
go mod download

# Install protobuf Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### 2. Generate Protobuf Code

```bash
# Generate gRPC code from .proto files
make proto

# Or manually:
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    internal/api/zenith.proto
```

### 3. Start CockroachDB

```bash
# Start CockroachDB using Docker Compose
docker-compose up -d

# Verify it's running
docker-compose ps

# Access the Admin UI at http://localhost:8080
```

### 4. Create Database

The server will create the table automatically, but you need to create the database first:

```bash
# Using Docker (recommended if you don't have CockroachDB CLI installed)
docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'

# Or if you have CockroachDB CLI installed
cockroach sql --insecure --host=localhost:26257 -e 'CREATE DATABASE IF NOT EXISTS zenith;'
```

**Note:** The server automatically runs table migrations on startup, so you only need to create the database.

### 5. Build and Run the Server

```bash
# Build the server
go build -o zenith ./cmd/server

# Run the server
./zenith

# Or with custom flags
./zenith -port=50051 -db="postgres://root@localhost:26257/zenith?sslmode=disable"
```

### 6. Test the Server

```bash
# Run the automated test script (starts server if needed)
./scripts/test.sh

# Or test manually with grpcurl
# First, ensure grpcurl is in your PATH:
export PATH="$(go env GOPATH)/bin:$PATH"

# Write a tuple
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

# Check the tuple
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

## API Usage

### gRPC Endpoints

#### Write

Inserts or deletes a relation tuple.

```protobuf
rpc Write(WriteRequest) returns (WriteResponse);

message WriteRequest {
  RelationTuple tuple = 1;
  WriteOperation operation = 2;  // INSERT or DELETE
}

message WriteResponse {
  int64 zookie = 1;  // Logical timestamp
}
```

#### Check

Evaluates whether a subject has a relation on an object.

```protobuf
rpc Check(CheckRequest) returns (CheckResponse);

message CheckRequest {
  string subject_namespace = 1;
  string subject_id = 2;
  string subject_relation = 3;  // Optional for usersets
  
  string namespace = 4;   // Object namespace
  string object_id = 5;   // Object ID
  string relation = 6;    // Relation to check
  
  int64 required_zookie = 7;  // Optional: ensure freshness
}

message CheckResponse {
  bool allowed = 1;
  int64 zookie = 2;
}
```

### Example: Using grpcurl

```bash
# Install grpcurl if needed
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Write a tuple: user "alice" has "viewer" relation on "doc_1"
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

# Check if user "alice" has "viewer" relation on "doc_1"
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

## Development

### Project Structure

```
zenith/
├── cmd/server/          # Application entry point
├── internal/
│   ├── api/            # gRPC service definitions (.proto)
│   ├── db/             # Database layer (connection, migrations, repository)
│   ├── models/         # Domain models
│   └── service/        # gRPC service implementation
├── docker-compose.yml  # Local CockroachDB setup
├── Dockerfile          # Application container
└── Makefile           # Build and generation tasks
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...
```

### Building with Docker

```bash
# Build the Docker image
docker build -t zenith:latest .

# Run the container
docker run -p 50051:50051 \
  -e DB_CONN_STR="postgres://root@host.docker.internal:26257/zenith?sslmode=disable" \
  zenith:latest
```

## Zookies (Causal Consistency)

Zenith uses "Zookies" (logical timestamps from CockroachDB) to ensure causal consistency:

1. **Write Operations**: Return a Zookie after inserting/deleting a tuple
2. **Check Operations**: Accept an optional `required_zookie` to ensure the check reads data at least as fresh as that timestamp
3. **AS OF SYSTEM TIME**: Queries use CockroachDB's `AS OF SYSTEM TIME` feature to prevent "time-travel" bugs

Example flow:
```go
// 1. Write a tuple, get zookie
writeResp, _ := client.Write(ctx, &WriteRequest{...})
zookie := writeResp.Zookie

// 2. Check with required_zookie to ensure we see the write
checkResp, _ := client.Check(ctx, &CheckRequest{
    ...,
    RequiredZookie: zookie,
})
```

## Project Status

✅ **Completed Features:**
- Database schema with indexes
- gRPC API definitions (Write, Check, ListSubjects)
- Write and Check endpoints with recursive expansion
- ListSubjects endpoint with reverse expansion
- Zookie support with AS OF SYSTEM TIME for causal consistency
- Recursive expansion engine with userset resolution
- Cycle detection and depth limiting
- Timeout handling for long-running expansions
- Caching system with negative caching and sub-graph caching
- Selective cache invalidation (only invalidates related entries)
- Nested userset expansion in reverse expansion
- Singleflight deduplication for concurrent requests
- OpenTelemetry tracing support
- Prometheus metrics and monitoring
- Docker setup for local development
- Comprehensive test coverage

🚀 **Production Ready:**
- Metrics endpoint on port 9090 (configurable via `-metrics-port`)
- Health check service
- Graceful shutdown
- Connection pooling
- Error handling with proper gRPC status codes

## Configuration

### Environment Variables

- `ZENITH_PORT`: gRPC server port (default: 50051)
- `ZENITH_DB`: Database connection string
- `ZENITH_ENABLE_REFLECTION`: Enable gRPC reflection (default: true)
- `ZENITH_METRICS_PORT`: HTTP port for Prometheus metrics (default: 9090)

### Command Line Flags

```bash
./zenith -port=50051 \
         -db="postgres://root@localhost:26257/zenith?sslmode=disable" \
         -reflection=true \
         -metrics-port=9090 \
         -enable-cache=true \
         -cache-size=10000 \
         -cache-ttl-positive=30s \
         -cache-ttl-negative=5s \
         -max-depth=10 \
         -check-timeout=10 \
         -enable-tracing=true \
         -tracing-endpoint=localhost:4317
```

### Monitoring

Zenith exposes Prometheus metrics on the metrics port (default: 9090):

```bash
# View metrics
curl http://localhost:9090/metrics
```

Available metrics:
- `zenith_requests_total`: Total requests by operation and status
- `zenith_request_duration_seconds`: Request duration histograms
- `zenith_cache_hits_total`: Cache hit counts by cache type
- `zenith_cache_misses_total`: Cache miss counts by cache type
- `zenith_expansion_depth`: Distribution of expansion depths
- `zenith_database_query_duration_seconds`: Database query durations
- `zenith_active_connections`: Number of active database connections

## Testing

See [TESTING.md](TESTING.md) for comprehensive testing instructions.

### Quick Start Testing

```bash
# 1. Start CockroachDB
docker-compose up -d

# 2. Create database
docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'

# 3. Build and run server
go build -o zenith ./cmd/server
./zenith

# 4. In another terminal, run tests
./scripts/test.sh
```

### Unit Tests

```bash
# Run all unit tests
go test ./...

# Run with coverage
go test -cover ./...
```

## Troubleshooting

### Protobuf Generation Errors

If you see errors about missing `internal/api` package:
1. Ensure `protoc` is installed: `protoc --version`
2. Run `make proto` to generate the code
3. Verify `internal/api/zenith.pb.go` and `internal/api/zenith_grpc.pb.go` exist

### Database Connection Issues

1. Verify CockroachDB is running: `docker-compose ps`
2. Check connection string format: `postgres://root@localhost:26257/zenith?sslmode=disable`
3. Test connection using Docker: `docker exec zenith-cockroachdb ./cockroach sql --insecure`
4. Create the database if missing: `docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'`

### Port Already in Use

Change the port: `./zenith -port=50052`

## License

[Add your license here]

