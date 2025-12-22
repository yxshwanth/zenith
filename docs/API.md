# Zenith API Documentation

## Overview

Zenith provides a unified gRPC API for permission checking and tuple management. All operations support causal consistency through Zookies (logical timestamps).

## Base URL

```
grpc://localhost:50051
```

## Authentication

Currently, Zenith does not require authentication. In production deployments, you should add authentication middleware.

## Endpoints

### Write

Inserts or deletes a relation tuple.

**Request:**
```protobuf
message WriteRequest {
  RelationTuple tuple = 1;
  WriteOperation operation = 2;  // INSERT or DELETE
}

message RelationTuple {
  string namespace = 1;        // Object type (e.g., "doc", "folder")
  string object_id = 2;        // Unique instance ID
  string relation = 3;         // Role (e.g., "owner", "viewer", "editor")
  string subject_namespace = 4; // Subject type ("user" or object namespace)
  string subject_id = 5;       // Subject identifier
  string subject_relation = 6;  // Optional: relation for usersets (e.g., "member")
}

enum WriteOperation {
  WRITE_OPERATION_UNSPECIFIED = 0;
  WRITE_OPERATION_INSERT = 1;
  WRITE_OPERATION_DELETE = 2;
}
```

**Response:**
```protobuf
message WriteResponse {
  int64 zookie = 1;  // Logical timestamp
}
```

**Example (grpcurl):**
```bash
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

**Response:**
```json
{
  "zookie": "1766433684599320881"
}
```

**Error Codes:**
- `INVALID_ARGUMENT`: Invalid tuple or operation
- `INTERNAL`: Database error

### Check

Evaluates whether a subject has a relation on an object. Supports recursive expansion through usersets.

**Request:**
```protobuf
message CheckRequest {
  string subject_namespace = 1;
  string subject_id = 2;
  string subject_relation = 3;  // Optional for usersets
  string namespace = 4;  // Object namespace
  string object_id = 5;  // Object ID
  string relation = 6;   // Relation to check
  int64 required_zookie = 7;  // Optional: ensure check is at least this fresh
}
```

**Response:**
```protobuf
message CheckResponse {
  bool allowed = 1;   // True if subject has the relation on object
  int64 zookie = 2;   // Logical timestamp of the check
}
```

**Example (grpcurl):**
```bash
grpcurl -plaintext -d '{
  "subject_namespace": "user",
  "subject_id": "alice",
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/Check
```

**Response:**
```json
{
  "allowed": true,
  "zookie": "1766433684599320882"
}
```

**Error Codes:**
- `INVALID_ARGUMENT`: Missing required fields
- `DEADLINE_EXCEEDED`: Operation timed out
- `INTERNAL`: Expansion or database error
- `RESOURCE_EXHAUSTED`: Rate limit exceeded

**Behavior:**
- Performs recursive expansion through usersets
- Detects cycles automatically
- Respects max depth and timeout limits
- Uses singleflight deduplication for concurrent identical requests
- Supports caching for performance

### ListSubjects

Returns all subjects that have a relation on an object (reverse expansion).

**Request:**
```protobuf
message ListSubjectsRequest {
  string namespace = 1;  // Object namespace
  string object_id = 2;  // Object ID
  string relation = 3;   // Relation to check
  int64 required_zookie = 4;  // Optional: ensure check is at least this fresh
}
```

**Response:**
```protobuf
message ListSubjectsResponse {
  repeated Subject subjects = 1;  // List of subjects with the relation
  int64 zookie = 2;               // Logical timestamp of the check
}

message Subject {
  string namespace = 1;  // Subject namespace (e.g., "user", "group")
  string id = 2;         // Subject identifier
  string relation = 3;    // Relation (empty for direct users, e.g., "member" for usersets)
}
```

**Example (grpcurl):**
```bash
grpcurl -plaintext -d '{
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/ListSubjects
```

**Response:**
```json
{
  "subjects": [
    {
      "namespace": "user",
      "id": "alice",
      "relation": ""
    },
    {
      "namespace": "user",
      "id": "bob",
      "relation": ""
    }
  ],
  "zookie": "1766433684599320883"
}
```

**Error Codes:**
- `INVALID_ARGUMENT`: Missing required fields
- `DEADLINE_EXCEEDED`: Operation timed out
- `INTERNAL`: Expansion or database error
- `RESOURCE_EXHAUSTED`: Rate limit exceeded

## Error Codes

### gRPC Status Codes

| Code | Description | When It Occurs |
|------|-------------|----------------|
| `OK` | Success | Operation completed successfully |
| `INVALID_ARGUMENT` | Invalid input | Missing or invalid request parameters |
| `NOT_FOUND` | Resource not found | Requested resource doesn't exist |
| `ALREADY_EXISTS` | Conflict | Resource already exists (on duplicate insert) |
| `DEADLINE_EXCEEDED` | Timeout | Operation exceeded timeout limit |
| `RESOURCE_EXHAUSTED` | Rate limited | Request rate limit exceeded |
| `INTERNAL` | Internal error | Database or expansion engine error |

### Error Details

Errors include detailed messages. For example:
- `INVALID_ARGUMENT: namespace is required`
- `DEADLINE_EXCEEDED: operation check timed out after 10ms`
- `RESOURCE_EXHAUSTED: rate limit exceeded: client limit`

## Causal Consistency (Zookies)

Zenith uses Zookies (logical timestamps from CockroachDB) to ensure causal consistency:

1. **Write Operations**: Return a Zookie after inserting/deleting a tuple
2. **Check Operations**: Accept an optional `required_zookie` to ensure the check reads data at least as fresh as that timestamp
3. **AS OF SYSTEM TIME**: Queries use CockroachDB's `AS OF SYSTEM TIME` feature to prevent "time-travel" bugs

**Example Flow:**
```bash
# 1. Write a tuple, get zookie
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

## Rate Limiting

If rate limiting is enabled, requests may be rejected with `RESOURCE_EXHAUSTED` status code.

Rate limits can be configured per-client (by IP or client ID) and globally. The `x-client-id` metadata header can be used to identify clients for per-client rate limiting.

## Best Practices

1. **Use Zookies for Consistency**: Always use `required_zookie` when checking permissions after a write to ensure you see the latest data.

2. **Handle Rate Limits**: Implement retry logic with exponential backoff for `RESOURCE_EXHAUSTED` errors.

3. **Cache Results**: The service caches check results. For high-frequency checks, consider caching on the client side as well.

4. **Batch Operations**: When possible, batch multiple checks in a single request (if supported in future versions).

5. **Monitor Expansion Depth**: Deeply nested usersets may cause timeouts. Monitor expansion depth in your traces.

6. **Use Appropriate Timeouts**: Set appropriate timeouts for Check operations based on your expected expansion depth.

## Examples

### Example 1: Direct User Permission

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

### Example 2: Nested Userset (2 levels)

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

### Example 3: List All Viewers

```bash
# List all subjects with "viewer" relation on "doc_1"
grpcurl -plaintext -d '{
  "namespace": "doc",
  "object_id": "doc_1",
  "relation": "viewer"
}' localhost:50051 zenith.v1.Zenith/ListSubjects
```

### Example 4: Delete Permission

```bash
# Delete a tuple
grpcurl -plaintext -d '{
  "tuple": {
    "namespace": "doc",
    "object_id": "doc_1",
    "relation": "viewer",
    "subject_namespace": "user",
    "subject_id": "alice"
  },
  "operation": "WRITE_OPERATION_DELETE"
}' localhost:50051 zenith.v1.Zenith/Write
```

## Performance Considerations

- **Latency**: Check operations typically complete in <10ms for direct lookups, <50ms for nested usersets (2-3 levels)
- **Throughput**: With caching enabled, Zenith can handle thousands of requests per second
- **Expansion Depth**: Deeper nesting increases latency. Monitor expansion depth metrics
- **Cache**: Enable caching for better performance. Cache hit rates are exposed via metrics

## Monitoring

Zenith exposes Prometheus metrics on port 9090 (configurable):

- `zenith_requests_total`: Total requests by operation and status
- `zenith_request_duration_seconds`: Request duration histograms
- `zenith_cache_hits_total`: Cache hit counts
- `zenith_cache_misses_total`: Cache miss counts
- `zenith_expansion_depth`: Distribution of expansion depths
- `zenith_database_query_duration_seconds`: Database query durations

Access metrics at: `http://localhost:9090/metrics`

