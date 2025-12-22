#!/bin/bash

set -e

echo "🚀 Setting up Zenith..."

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "❌ protoc is not installed"
    echo "   Please install it:"
    echo "   - macOS: brew install protobuf"
    echo "   - Linux: apt-get install protobuf-compiler"
    exit 1
fi

echo "✅ protoc found: $(protoc --version)"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed"
    exit 1
fi

echo "✅ Go found: $(go version)"

# Install protobuf plugins
echo "📦 Installing protobuf Go plugins..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Add Go bin directory to PATH if not already there
GOPATH_BIN="$(go env GOPATH)/bin"
if [[ ":$PATH:" != *":$GOPATH_BIN:"* ]]; then
    export PATH="$GOPATH_BIN:$PATH"
fi

# Verify plugins are installed
if ! command -v protoc-gen-go &> /dev/null; then
    echo "❌ protoc-gen-go not found in PATH"
    echo "   Please add $(go env GOPATH)/bin to your PATH"
    exit 1
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "❌ protoc-gen-go-grpc not found in PATH"
    echo "   Please add $(go env GOPATH)/bin to your PATH"
    exit 1
fi

echo "✅ Protobuf plugins installed"

# Generate protobuf code
echo "🔨 Generating protobuf code..."
make proto

# Download Go dependencies
echo "📥 Downloading Go dependencies..."
go mod download
go mod tidy

# Check if Docker is running
if ! docker info &> /dev/null; then
    echo "⚠️  Docker is not running. Please start Docker to use docker-compose."
else
    echo "✅ Docker is running"
fi

echo ""
echo "✅ Setup complete!"
echo ""
echo "Next steps:"
echo "1. Start CockroachDB: docker-compose up -d"
echo "2. Create the database: docker exec zenith-cockroachdb ./cockroach sql --insecure -e 'CREATE DATABASE IF NOT EXISTS zenith;'"
echo "3. Build the server: go build -o zenith ./cmd/server"
echo "4. Run the server: ./zenith"

