#!/bin/bash

# Quick test using grpcurl with PATH fix
export PATH="$(go env GOPATH)/bin:$PATH"

# Check if server is running
if ! nc -z localhost 50051 2>/dev/null; then
    echo "⚠️  Server not running on port 50051"
    echo "Please start the server first: ./zenith"
    exit 1
fi

echo "🧪 Quick test - Writing and checking a tuple..."
echo ""

# Write
echo "Writing tuple..."
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

echo ""
echo "Checking tuple..."
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check

echo ""
echo "✅ Test complete!"

