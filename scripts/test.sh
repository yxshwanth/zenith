#!/bin/bash

# Test script for Zenith Phase 1
# This script tests the Write and Check endpoints

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Add Go bin to PATH
export PATH="$(go env GOPATH)/bin:$PATH"

# Check if grpcurl is available
if ! command -v grpcurl &> /dev/null; then
    echo -e "${YELLOW}⚠️  grpcurl not found. Installing...${NC}"
    go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
    export PATH="$(go env GOPATH)/bin:$PATH"
fi

SERVER="${1:-localhost:50051}"

echo -e "${GREEN}🧪 Testing Zenith Phase 1${NC}"
echo "Server: $SERVER"
echo ""

# Test 1: Write a tuple
echo -e "${GREEN}Test 1: Writing tuple (user:alice -> doc:doc_1#viewer)${NC}"
WRITE_RESPONSE=$(grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "user",
    "subject_id": "alice"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' "$SERVER" zenith.v1.Zenith/Write)

echo "Response: $WRITE_RESPONSE"
ZOOKIE=$(echo "$WRITE_RESPONSE" | grep -oE '"zookie":\s*"[0-9]+"' | grep -oE '[0-9]+' | head -1)
echo -e "${GREEN}✅ Write successful. Zookie: $ZOOKIE${NC}"
echo ""

# Test 2: Check the tuple (should return true)
echo -e "${GREEN}Test 2: Checking tuple (user:alice -> doc:doc_1#viewer)${NC}"
CHECK_RESPONSE=$(grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' "$SERVER" zenith.v1.Zenith/Check)

echo "Response: $CHECK_RESPONSE"
if echo "$CHECK_RESPONSE" | grep -qE '"allowed":\s*true'; then
    echo -e "${GREEN}✅ Check successful - permission granted${NC}"
else
    echo -e "${RED}❌ Check failed - permission denied${NC}"
    exit 1
fi
echo ""

# Test 3: Check non-existent tuple (should return false)
echo -e "${GREEN}Test 3: Checking non-existent tuple (user:bob -> doc:doc_1#viewer)${NC}"
CHECK_RESPONSE2=$(grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "bob",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' "$SERVER" zenith.v1.Zenith/Check)

echo "Response: $CHECK_RESPONSE2"
# Check if allowed is false or missing (protobuf omits false booleans in JSON)
if echo "$CHECK_RESPONSE2" | grep -qE '"allowed":\s*false' || ! echo "$CHECK_RESPONSE2" | grep -q '"allowed"'; then
    echo -e "${GREEN}✅ Check successful - permission correctly denied${NC}"
else
    echo -e "${RED}❌ Check failed - should have returned false${NC}"
    exit 1
fi
echo ""

# Test 4: Write a userset tuple
echo -e "${GREEN}Test 4: Writing userset tuple (group:eng#member -> doc:doc_1#viewer)${NC}"
WRITE_RESPONSE2=$(grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "group",
    "subject_id": "eng",
    "subject_relation": "member"
  },
  "operation": "WRITE_OPERATION_INSERT"
}' "$SERVER" zenith.v1.Zenith/Write)

echo "Response: $WRITE_RESPONSE2"
echo -e "${GREEN}✅ Userset tuple written${NC}"
echo ""

# Test 5: Check userset (Phase 1: direct check only, should return true)
echo -e "${GREEN}Test 5: Checking userset (group:eng#member -> doc:doc_1#viewer)${NC}"
CHECK_RESPONSE3=$(grpcurl -plaintext -d '{
  "subject_namespace": "group",
  "subject_id": "eng",
  "subject_relation": "member",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' "$SERVER" zenith.v1.Zenith/Check)

echo "Response: $CHECK_RESPONSE3"
if echo "$CHECK_RESPONSE3" | grep -qE '"allowed":\s*true'; then
    echo -e "${GREEN}✅ Userset check successful${NC}"
else
    echo -e "${RED}❌ Userset check failed${NC}"
    exit 1
fi
echo ""

echo -e "${GREEN}🎉 All tests passed!${NC}"
echo ""
echo -e "${YELLOW}Note: Phase 1 only supports direct tuple matching.${NC}"
echo -e "${YELLOW}Phase 2 will add recursive expansion for nested usersets.${NC}"

