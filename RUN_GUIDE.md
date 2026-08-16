# Zenith - Run Guide

This guide will help you run both the backend and frontend of the Zenith permission system.

## Prerequisites

Before starting, ensure you have:

1. **Go 1.21+** installed
2. **Node.js 18+** and npm/yarn/pnpm installed
3. **Docker** and Docker Compose installed
4. **Protocol Buffers compiler** (`protoc`) - Optional, only if you need to regenerate protobuf code

## Quick Start

### Step 1: Start the Database

Zenith uses CockroachDB. Start it using Docker Compose:

```bash
docker-compose up -d
```

This will start CockroachDB on:
- **SQL Port**: `26257`
- **Admin UI**: `http://localhost:8080`

### Step 2: Create the Database

Create the `zenith` database:

```bash
docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'
```

### Step 3: Generate Protobuf Code (if needed)

If you haven't already generated the protobuf code:

```bash
make proto
```

Or manually:
```bash
protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    internal/api/zenith.proto
```

### Step 4: Create Configuration File

Create a configuration file. You can use either YAML or JSON format:

**Option A: Copy the example YAML config:**
```bash
cp config.example.yaml config.yaml
```

**Option B: Copy the example JSON config:**
```bash
cp config.example.json config.json
```

The default configuration expects:
- Database: `postgres://root@localhost:26257/zenith?sslmode=disable`
- gRPC Server: Port `50051`
- HTTP Gateway: Port `8081` (for REST API) - Note: Changed from 8080 to avoid conflict with CockroachDB Admin UI
- Metrics: Port `9090`

### Step 5: Build the Backend

```bash
go build -o zenith ./cmd/server
```

### Step 6: Run the Backend

```bash
./zenith -config=config.yaml
```

Or if using JSON:
```bash
./zenith -config=config.json
```

You should see output like:
```
Connecting to database...
Database connection established
Running migrations...
Migrations completed
Cache initialized: size=10000, ttl-positive=30s, ttl-negative=5s
Expansion engine initialized: max-depth=10, timeout=10ms
Starting metrics server on port 9090...
Starting HTTP gateway on port 8081...
Starting gRPC server on port 50051...
```

The backend is now running on:
- **gRPC API**: `localhost:50051`
- **HTTP Gateway (REST API)**: `http://localhost:8081`
- **Metrics**: `http://localhost:9090/metrics`

### Step 7: Install Frontend Dependencies

Open a new terminal and navigate to the frontend directory:

```bash
cd frontend
npm install
```

### Step 8: Run the Frontend

```bash
npm run dev
```

The frontend will start on `http://localhost:3000`

### Step 9: View the Application

Open your browser and navigate to:
- **Frontend UI**: `http://localhost:3000`

## Port Summary

| Service | Port | URL |
|---------|------|-----|
| Frontend | 3000 | http://localhost:3000 |
| Backend gRPC | 50051 | localhost:50051 |
| Backend HTTP Gateway | 8081 | http://localhost:8081 |
| Metrics | 9090 | http://localhost:9090/metrics |
| CockroachDB SQL | 26257 | localhost:26257 |
| CockroachDB Admin UI | 8080 | http://localhost:8080 |

**Note**: CockroachDB Admin UI uses port 8080, and the HTTP Gateway uses port 8081 to avoid conflicts.

## Troubleshooting

### Backend won't start

1. **Database connection error**: Make sure CockroachDB is running:
   ```bash
   docker ps | grep cockroach
   ```

2. **Port already in use**: Check if ports 50051, 8081, or 9090 are already in use:
   ```bash
   lsof -i :50051
   lsof -i :8081
   lsof -i :9090
   ```

3. **Database doesn't exist**: Create it:
   ```bash
   docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'
   ```

### Frontend won't connect to backend

1. **Backend not running**: Make sure the backend is running and the HTTP gateway is started (check logs for "Starting HTTP gateway on port 8081...")

2. **CORS issues**: The HTTP gateway includes CORS middleware, but if you see CORS errors, check that the backend is running on port 8081

3. **Proxy configuration**: The frontend Vite proxy is configured to forward `/api/*` requests to `http://localhost:8081/api/*`. Make sure the backend HTTP gateway is running.

### Database migration errors

If you see migration errors, you can reset the database:

```bash
docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'DROP DATABASE IF EXISTS zenith; CREATE DATABASE zenith;'
```

Then restart the backend.

## Testing the Setup

### Test Backend (gRPC)

Using `grpcurl`:

```bash
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

# Check permission
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

### Test Backend (HTTP Gateway)

Using `curl`:

```bash
# Write a tuple
curl -X POST http://localhost:8081/api/tuples \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "user",
    "subject_id": "alice"
  }'

# Check permission
curl -X POST http://localhost:8081/api/check \
  -H "Content-Type: application/json" \
  -d '{
    "subject_namespace": "user",
    "subject_id": "alice",
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer"
  }'

# List all tuples
curl http://localhost:8081/api/tuples
```

### Test Frontend

1. Open `http://localhost:3000` in your browser
2. Navigate to different sections:
   - **Permission Graph**: Visualize permission relationships
   - **Permission Checker**: Check individual permissions
   - **Batch Checker**: Test batch operations
   - **Tuple Manager**: Create, read, update, delete tuples
   - **Performance Dashboard**: View metrics and performance data
   - **Temporal Audit**: View permissions at different points in time

## Development Workflow

### Running in Development Mode

1. **Backend**: Run `./zenith -config=config.yaml` (it will auto-reload on code changes if using a tool like `air` or `nodemon`)

2. **Frontend**: Run `npm run dev` (Vite will hot-reload on file changes)

### Building for Production

**Backend:**
```bash
go build -o zenith ./cmd/server
```

**Frontend:**
```bash
cd frontend
npm run build
```

The production build will be in `frontend/dist/`

## Stopping Services

1. **Stop Frontend**: Press `Ctrl+C` in the frontend terminal

2. **Stop Backend**: Press `Ctrl+C` in the backend terminal

3. **Stop Database**:
   ```bash
   docker-compose down
   ```

   To also remove volumes (deletes all data):
   ```bash
   docker-compose down -v
   ```

## Additional Configuration

### Changing Ports

Edit `config.yaml` or `config.json`:

```yaml
server:
  port: 50051          # gRPC port
  metrics_port: 9090   # Metrics port
  http_port: 8081      # HTTP gateway port (8080 is used by CockroachDB Admin UI)
```

### Environment Variables

You can override config with environment variables:

```bash
export ZENITH_SERVER_PORT=50051
export ZENITH_SERVER_HTTP_PORT=8081
export ZENITH_DATABASE_CONNECTION_STRING="postgres://root@localhost:26257/zenith?sslmode=disable"
./zenith
```

### Command-Line Flags

Override any config with flags:

```bash
./zenith -config=config.yaml -port=50051 -http-port=8081
```

## Next Steps

- Read the [API Documentation](docs/API.md) for detailed API usage
- Check [Development Guide](docs/DEVELOPMENT.md) for contributing
- Review [Architecture](docs/ARCHITECTURE.md) for system design
- See [Performance Guide](docs/PERFORMANCE.md) for optimization tips

