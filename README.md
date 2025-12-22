# Zenith - Production-Ready Permission System

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**Zenith** is a high-performance, production-ready permission system that provides a unified API to answer: *"Does subject S have relation R on object O?"* 

Inspired by Google's [Zanzibar](https://research.google/pubs/zanzibar-googles-consistent-global-authorization-system/), Zenith handles complex permission scenarios including deep nesting (e.g., user is in a group, which is in another group, which has access to a folder) while maintaining sub-10ms latency for direct checks.

## 🚀 Features

### Core Capabilities

- **Unified Permission Model**: Relation tuples stored as a Directed Acyclic Graph (DAG)
- **Recursive Expansion**: Automatic userset resolution with cycle detection
- **Causal Consistency**: Zookie-based consistency using CockroachDB logical timestamps
- **High Performance**: Sub-10ms latency for direct checks, <50ms for nested expansion
- **Production Ready**: Enterprise-grade features including monitoring, rate limiting, and comprehensive error handling

### Advanced Features

- ✅ **Recursive Expansion Engine**: Handles deeply nested usersets with automatic cycle detection
- ✅ **Reverse Expansion**: List all subjects with a relation on an object
- ✅ **Intelligent Caching**: LRU cache with negative caching and sub-graph caching
- ✅ **Selective Cache Invalidation**: Only invalidates related entries, not entire cache
- ✅ **Singleflight Deduplication**: Prevents duplicate concurrent requests
- ✅ **Rate Limiting**: Token bucket algorithm with per-client and global limits
- ✅ **Configuration Management**: YAML/JSON files, environment variables, and command-line flags
- ✅ **Enhanced Error Handling**: Custom error types with proper gRPC status codes
- ✅ **Connection Pooling**: Configurable pool with health checks and automatic reconnection
- ✅ **Observability**: OpenTelemetry tracing and Prometheus metrics
- ✅ **Comprehensive Testing**: Unit tests, integration tests, and performance benchmarks

## 📋 Table of Contents

- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Installation](#installation)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Examples](#examples)
- [Performance](#performance)
- [Monitoring](#monitoring)
- [Documentation](#documentation)
- [Development](#development)
- [Deployment](#deployment)
- [Contributing](#contributing)

## 🏃 Quick Start

### Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- Protocol Buffers compiler (`protoc`)

### 5-Minute Setup

```bash
# 1. Clone the repository
git clone https://github.com/yxshwanth/zenith.git
cd zenith

# 2. Install dependencies
go mod download
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 3. Generate protobuf code
make proto

# 4. Start CockroachDB
docker-compose up -d

# 5. Create database
docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'

# 6. Build and run
go build -o zenith ./cmd/server
./zenith

# 7. Test it (in another terminal)
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
```

## 🏗️ Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────┐
│                    gRPC API Layer                        │
│  (Write, Check, ListSubjects with Rate Limiting)       │
└────────────────────┬────────────────────────────────────┘
                     │
         ┌───────────┴───────────┐
         │                       │
    ┌────▼────┐            ┌────▼────┐
    │ Service │            │  Cache  │
    │  Layer  │◄───────────┤  Layer  │
    └────┬────┘            └─────────┘
         │
    ┌────▼────────────────────┐
    │   Expansion Engines     │
    │  (Forward & Reverse)    │
    └────┬────────────────────┘
         │
    ┌────▼────┐
    │Database │
    │  Layer  │
    └────┬────┘
         │
    ┌────▼────┐
    │Cockroach│
    │   DB    │
    └─────────┘
```

### Data Model

Every permission is stored as a **relation tuple**:

```
namespace:object_id#relation@subject_namespace:subject_id#subject_relation
```

**Example:**
```
doc:doc_1#viewer@user:alice
```
*"User alice has viewer relation on document doc_1"*

```
doc:doc_1#viewer@group:eng#member
```
*"Members of group eng have viewer relation on document doc_1"*

### Key Components

1. **Service Layer** (`internal/service/`): gRPC service implementation with request validation and deduplication
2. **Expansion Engine** (`internal/engine/`): Recursive userset expansion with cycle detection
3. **Database Layer** (`internal/db/`): CockroachDB integration with Zookie support
4. **Cache Layer** (`internal/cache/`): Intelligent caching with selective invalidation
5. **Configuration** (`internal/config/`): Multi-source configuration management
6. **Middleware** (`internal/middleware/`): Rate limiting and future middleware
7. **Observability** (`internal/observability/`): Tracing and metrics

## 📦 Installation

### From Source

```bash
git clone https://github.com/yxshwanth/zenith.git
cd zenith
go mod download
make proto
go build -o zenith ./cmd/server
```

### Using Docker

```bash
docker build -t zenith:latest .
docker run -p 50051:50051 -p 9090:9090 \
  -e ZENITH_DATABASE_CONNECTION_STRING="postgres://root@host.docker.internal:26257/zenith?sslmode=disable" \
  zenith:latest
```

## ⚙️ Configuration

Zenith supports multiple configuration sources with priority: **flags > environment variables > config file > defaults**

### Configuration File (YAML/JSON)

Create `config.yaml`:

```yaml
server:
  port: 50051
  metrics_port: 9090
  reflection: true

database:
  connection_string: "postgres://root@localhost:26257/zenith?sslmode=disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  health_check_interval: 30s

cache:
  enabled: true
  size: 10000
  ttl_positive: 30s
  ttl_negative: 5s

engine:
  max_depth: 10
  check_timeout_ms: 10

tracing:
  enabled: true
  endpoint: "localhost:4317"

rate_limit:
  enabled: false
  global_rps: 1000
  per_client_rps: 100
  burst_size: 10
```

Run with config file:
```bash
./zenith -config=config.yaml
```

### Environment Variables

All settings can be configured via environment variables with `ZENITH_` prefix:

```bash
export ZENITH_SERVER_PORT=50051
export ZENITH_DATABASE_CONNECTION_STRING="postgres://..."
export ZENITH_CACHE_ENABLED=true
export ZENITH_RATE_LIMIT_ENABLED=true
./zenith
```

### Command-Line Flags

Flags override config file and environment variables:

```bash
./zenith \
  -config=config.yaml \
  -port=50051 \
  -db="postgres://root@localhost:26257/zenith?sslmode=disable" \
  -enable-cache=true \
  -cache-size=50000 \
  -rate-limit-enabled=true \
  -metrics-port=9090
```

See `config.example.yaml` and `config.example.json` for complete examples.

## 📡 API Reference

Zenith provides three gRPC endpoints:

### Write

Inserts or deletes a relation tuple.

**Request:**
```protobuf
message WriteRequest {
  RelationTuple tuple = 1;
  WriteOperation operation = 2;  // INSERT or DELETE
}
```

**Response:**
```protobuf
message WriteResponse {
  int64 zookie = 1;  // Logical timestamp
}
```

### Check

Evaluates whether a subject has a relation on an object. Supports recursive expansion.

**Request:**
```protobuf
message CheckRequest {
  string subject_namespace = 1;
  string subject_id = 2;
  string subject_relation = 3;  // Optional for usersets
  string namespace = 4;
  string object_id = 5;
  string relation = 6;
  int64 required_zookie = 7;  // Optional: ensure freshness
}
```

**Response:**
```protobuf
message CheckResponse {
  bool allowed = 1;
  int64 zookie = 2;
}
```

### ListSubjects

Returns all subjects that have a relation on an object (reverse expansion).

**Request:**
```protobuf
message ListSubjectsRequest {
  string namespace = 1;
  string object_id = 2;
  string relation = 3;
  int64 required_zookie = 4;
}
```

**Response:**
```protobuf
message ListSubjectsResponse {
  repeated Subject subjects = 1;
  int64 zookie = 2;
}
```

For complete API documentation, see [docs/API.md](docs/API.md).

## 💡 Examples

### Example 1: Direct Permission

```bash
# Write: user "alice" has "viewer" relation on "doc_1"
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

# Check: Does user "alice" have "viewer" on "doc_1"?
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

### Example 2: Nested Usersets (2 levels)

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

# 3. Check if user has access (recursively expands)
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "bob",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
# Returns: {"allowed": true}
```

### Example 3: List All Viewers

```bash
# List all subjects with "viewer" relation on "doc_1"
grpcurl -plaintext -d '{
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/ListSubjects
```

## ⚡ Performance

Zenith is optimized for high performance:

- **Direct Checks**: <5ms (cache hit) to <10ms (cache miss)
- **2-Level Expansion**: 10-20ms
- **3-Level Expansion**: 20-40ms
- **Throughput**: 5,000-10,000 requests/second (with caching)

### Benchmark Results

```
BenchmarkCheck_Direct-8         1,031,558 ops    1,189 ns/op
BenchmarkCheck_Nested-8           160,956 ops    7,373 ns/op
BenchmarkExpansion_2Level-8        249,531 ops    4,716 ns/op
BenchmarkExpansion_3Level-8        162,390 ops    7,384 ns/op
```

For detailed performance information, see [docs/PERFORMANCE.md](docs/PERFORMANCE.md).

## 📊 Monitoring

Zenith exposes Prometheus metrics on port 9090 (configurable):

```bash
curl http://localhost:9090/metrics
```

### Key Metrics

- `zenith_requests_total`: Total requests by operation and status
- `zenith_request_duration_seconds`: Request duration histograms
- `zenith_cache_hits_total` / `zenith_cache_misses_total`: Cache performance
- `zenith_expansion_depth`: Distribution of expansion depths
- `zenith_database_query_duration_seconds`: Database query performance
- `zenith_active_connections`: Database connection pool status

### Grafana Dashboard

Import the metrics into Grafana for visualization. Key queries:

- **Request Rate**: `rate(zenith_requests_total[5m])`
- **P95 Latency**: `histogram_quantile(0.95, rate(zenith_request_duration_seconds_bucket[5m]))`
- **Cache Hit Rate**: `rate(zenith_cache_hits_total[5m]) / (rate(zenith_cache_hits_total[5m]) + rate(zenith_cache_misses_total[5m]))`

## 📚 Documentation

Comprehensive documentation is available in the `docs/` directory:

- **[API.md](docs/API.md)**: Complete API reference with examples
- **[DEVELOPMENT.md](docs/DEVELOPMENT.md)**: Development guide, architecture, and contribution guidelines
- **[DEPLOYMENT.md](docs/DEPLOYMENT.md)**: Production deployment guide with Docker, Kubernetes, and monitoring
- **[PERFORMANCE.md](docs/PERFORMANCE.md)**: Performance characteristics, benchmarks, and optimization tips
- **[TESTING.md](TESTING.md)**: Testing guide and examples

## 🛠️ Development

### Project Structure

```
zenith/
├── cmd/server/          # Application entry point
├── internal/
│   ├── api/            # gRPC service definitions (.proto)
│   ├── cache/          # Caching implementation
│   ├── config/         # Configuration management
│   ├── db/             # Database layer
│   ├── engine/         # Expansion engines
│   ├── errors/         # Custom error types
│   ├── metrics/        # Prometheus metrics
│   ├── middleware/     # gRPC middleware
│   ├── models/         # Domain models
│   ├── observability/  # Tracing
│   └── service/        # gRPC service implementation
├── docs/               # Documentation
├── scripts/            # Utility scripts
├── config.example.yaml # Example configuration
└── docker-compose.yml  # Local development setup
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. -benchmem ./...

# Run profiling script
./scripts/profile.sh
```

### Building

```bash
# Build binary
go build -o zenith ./cmd/server

# Build with Docker
docker build -t zenith:latest .
```

For detailed development instructions, see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## 🚢 Deployment

### Docker

```bash
docker run -d \
  --name zenith \
  -p 50051:50051 \
  -p 9090:9090 \
  -e ZENITH_DATABASE_CONNECTION_STRING="postgres://..." \
  zenith:latest
```

### Kubernetes

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for complete Kubernetes manifests and deployment strategies.

### Production Checklist

- [ ] Database configured with proper credentials
- [ ] Configuration file created and validated
- [ ] Monitoring configured (Prometheus, Grafana)
- [ ] Rate limiting configured appropriately
- [ ] Health checks configured
- [ ] Backup strategy in place
- [ ] Security review completed

For complete deployment guide, see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## 🔒 Security

- **Input Validation**: All inputs are validated
- **Rate Limiting**: Protection against DDoS and abuse
- **Error Handling**: No sensitive information in error messages
- **Connection Security**: Use TLS for database connections in production
- **Authentication**: Add authentication middleware for production (not included by default)

## 🧪 Testing

```bash
# Unit tests
go test ./...

# Integration tests (requires running database)
./scripts/test.sh

# Performance benchmarks
go test -bench=. -benchmem ./internal/service
```

See [TESTING.md](TESTING.md) for comprehensive testing guide.

## 🤝 Contributing

Contributions are welcome! Please see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for:

- Code style guidelines
- Testing requirements
- Pull request process
- Architecture overview

## 📄 License

[Add your license here]

## 🙏 Acknowledgments

- Inspired by [Google Zanzibar](https://research.google/pubs/zanzibar-googles-consistent-global-authorization-system/)
- Built with [CockroachDB](https://www.cockroachlabs.com/) for distributed consistency
- Uses [gRPC](https://grpc.io/) for efficient API communication

## 📞 Support

- **Documentation**: See `docs/` directory
- **Issues**: [GitHub Issues](https://github.com/yxshwanth/zenith/issues)
- **Examples**: See `docs/API.md` for API examples

---

**Zenith** - Production-ready permission system for modern applications.
