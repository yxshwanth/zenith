#!/usr/bin/env bash
# Seed CockroachDB and print Check latency (SQL microbench + gRPC load).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DSN="${ZENITH_DATABASE_CONNECTION_STRING:-postgres://root@localhost:26257/zenith?sslmode=disable}"
GRPC="${ZENITH_GRPC:-localhost:50051}"

if ! docker ps --format '{{.Names}}' | grep -q '^zenith-cockroachdb$'; then
  echo "starting cockroach..."
  docker compose up -d
  sleep 3
fi

docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;' >/dev/null

echo "=== SQL + seed ==="
go run ./cmd/bench -mode=seed -dsn="$DSN"
go run ./cmd/bench -mode=sql -dsn="$DSN"

if ! nc -z localhost 50051 2>/dev/null; then
  echo
  echo "zenith is not listening on :50051"
  echo "start it in another terminal, then re-run load:"
  echo "  go build -o zenith ./cmd/server"
  echo "  ./zenith -config=config.yaml -check-timeout=50 -enable-tracing=false"
  echo "  go run ./cmd/bench -mode=load -grpc=$GRPC"
  exit 0
fi

echo
echo "=== gRPC load ==="
go run ./cmd/bench -mode=load -grpc="$GRPC"
