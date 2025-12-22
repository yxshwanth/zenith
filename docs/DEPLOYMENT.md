# Zenith Deployment Guide

## Production Deployment Checklist

- [ ] Database configured and accessible
- [ ] Configuration file created and validated
- [ ] Environment variables set
- [ ] Monitoring configured (Prometheus, Grafana)
- [ ] Logging configured
- [ ] Health checks configured
- [ ] Rate limiting configured
- [ ] Backup strategy in place
- [ ] Security review completed
- [ ] Load testing performed

## Docker Deployment

### Build Image

```bash
docker build -t zenith:latest .
```

### Run Container

```bash
docker run -d \
  --name zenith \
  -p 50051:50051 \
  -p 9090:9090 \
  -e ZENITH_DATABASE_CONNECTION_STRING="postgres://user:pass@host:26257/zenith?sslmode=require" \
  -e ZENITH_SERVER_PORT=50051 \
  -e ZENITH_METRICS_PORT=9090 \
  -e ZENITH_CACHE_ENABLED=true \
  -e ZENITH_RATE_LIMIT_ENABLED=true \
  zenith:latest
```

### Using Docker Compose

Create `docker-compose.prod.yml`:

```yaml
version: '3.8'

services:
  zenith:
    build: .
    ports:
      - "50051:50051"
      - "9090:9090"
    environment:
      - ZENITH_DATABASE_CONNECTION_STRING=postgres://user:pass@cockroachdb:26257/zenith?sslmode=require
      - ZENITH_CACHE_ENABLED=true
      - ZENITH_RATE_LIMIT_ENABLED=true
    depends_on:
      - cockroachdb
    restart: unless-stopped

  cockroachdb:
    image: cockroachdb/cockroach:latest
    command: start-single-node --insecure
    volumes:
      - cockroach-data:/cockroach/cockroach-data
    ports:
      - "26257:26257"
      - "8080:8080"

volumes:
  cockroach-data:
```

Run:
```bash
docker-compose -f docker-compose.prod.yml up -d
```

## Kubernetes Deployment

### Deployment Manifest

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zenith
  labels:
    app: zenith
spec:
  replicas: 3
  selector:
    matchLabels:
      app: zenith
  template:
    metadata:
      labels:
        app: zenith
    spec:
      containers:
      - name: zenith
        image: zenith:latest
        ports:
        - containerPort: 50051
          name: grpc
        - containerPort: 9090
          name: metrics
        env:
        - name: ZENITH_DATABASE_CONNECTION_STRING
          valueFrom:
            secretKeyRef:
              name: zenith-secrets
              key: database-url
        - name: ZENITH_CACHE_ENABLED
          value: "true"
        - name: ZENITH_RATE_LIMIT_ENABLED
          value: "true"
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 9090
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 9090
          initialDelaySeconds: 10
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: zenith
spec:
  selector:
    app: zenith
  ports:
  - name: grpc
    port: 50051
    targetPort: 50051
  - name: metrics
    port: 9090
    targetPort: 9090
  type: LoadBalancer
```

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: zenith-config
data:
  config.yaml: |
    server:
      port: 50051
      metrics_port: 9090
    database:
      max_open_conns: 50
      max_idle_conns: 10
    cache:
      enabled: true
      size: 50000
    rate_limit:
      enabled: true
      global_rps: 5000
      per_client_rps: 500
```

### Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: zenith-secrets
type: Opaque
stringData:
  database-url: "postgres://user:password@cockroachdb:26257/zenith?sslmode=require"
```

## Environment Configuration

### Required Environment Variables

- `ZENITH_DATABASE_CONNECTION_STRING`: Database connection string

### Optional Environment Variables

- `ZENITH_SERVER_PORT`: gRPC server port (default: 50051)
- `ZENITH_SERVER_METRICS_PORT`: Metrics port (default: 9090)
- `ZENITH_CACHE_ENABLED`: Enable caching (default: true)
- `ZENITH_CACHE_SIZE`: Cache size (default: 10000)
- `ZENITH_RATE_LIMIT_ENABLED`: Enable rate limiting (default: false)
- `ZENITH_RATE_LIMIT_GLOBAL_RPS`: Global requests per second (default: 1000)
- `ZENITH_RATE_LIMIT_PER_CLIENT_RPS`: Per-client RPS (default: 100)
- `ZENITH_ENGINE_MAX_DEPTH`: Max expansion depth (default: 10)
- `ZENITH_ENGINE_CHECK_TIMEOUT_MS`: Check timeout in ms (default: 10)
- `ZENITH_TRACING_ENABLED`: Enable tracing (default: true)
- `ZENITH_TRACING_ENDPOINT`: OTel collector endpoint (default: localhost:4317)

## Monitoring Setup

### Prometheus Configuration

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'zenith'
    static_configs:
      - targets: ['zenith:9090']
    metrics_path: '/metrics'
```

### Grafana Dashboard

Key metrics to monitor:
- Request rate and latency
- Cache hit/miss rates
- Expansion depth distribution
- Database connection pool stats
- Rate limit rejections
- Error rates by type

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: zenith
    rules:
      - alert: HighErrorRate
        expr: rate(zenith_requests_total{status="error"}[5m]) > 0.1
        for: 5m
        annotations:
          summary: "High error rate in Zenith"
      
      - alert: HighLatency
        expr: histogram_quantile(0.99, zenith_request_duration_seconds) > 0.1
        for: 5m
        annotations:
          summary: "High latency in Zenith requests"
      
      - alert: DatabaseConnectionIssues
        expr: zenith_active_connections == 0
        for: 1m
        annotations:
          summary: "No active database connections"
```

## Logging Setup

Zenith uses standard Go logging. For production, consider:

1. **Structured Logging**: Use a library like `zerolog` or `zap`
2. **Log Aggregation**: Send logs to ELK, Loki, or similar
3. **Log Levels**: Configure appropriate log levels
4. **Request IDs**: Add request IDs for tracing

## Backup and Recovery

### Database Backups

CockroachDB backup strategy:

```bash
# Full backup
cockroach sql --insecure -e "BACKUP DATABASE zenith TO 's3://bucket/backup';"

# Incremental backup
cockroach sql --insecure -e "BACKUP DATABASE zenith TO 's3://bucket/backup' AS OF SYSTEM TIME '-1h';"
```

### Recovery

```bash
# Restore from backup
cockroach sql --insecure -e "RESTORE DATABASE zenith FROM 's3://bucket/backup';"
```

## Scaling Considerations

### Horizontal Scaling

- **Stateless Service**: Zenith is stateless (except cache), can scale horizontally
- **Load Balancer**: Use gRPC-aware load balancer (e.g., Envoy, nginx)
- **Database**: CockroachDB supports horizontal scaling
- **Cache**: Consider distributed cache (Redis) for multi-instance deployments

### Vertical Scaling

- **Memory**: Increase for larger cache sizes
- **CPU**: More CPU for higher throughput
- **Database Connections**: Adjust pool size based on load

### Performance Tuning

1. **Cache Size**: Increase for better hit rates
2. **Connection Pool**: Tune based on database capacity
3. **Expansion Depth**: Reduce if experiencing timeouts
4. **Rate Limits**: Adjust based on expected load

## Security Best Practices

1. **Database Security**:
   - Use TLS for database connections
   - Use strong passwords
   - Limit database access

2. **Network Security**:
   - Use TLS for gRPC (mTLS recommended)
   - Restrict access to metrics endpoint
   - Use firewall rules

3. **Authentication**:
   - Add authentication middleware (not yet implemented)
   - Use API keys or OAuth2

4. **Secrets Management**:
   - Use secret management (Vault, AWS Secrets Manager)
   - Never commit secrets to code
   - Rotate secrets regularly

5. **Input Validation**:
   - Validate all inputs
   - Sanitize user data
   - Set request size limits

## Performance Tuning

### Database Optimization

- **Indexes**: Ensure indexes are created (done automatically)
- **Connection Pool**: Tune based on load
- **Query Timeout**: Set appropriate timeouts

### Cache Optimization

- **Size**: Increase for better hit rates
- **TTL**: Adjust based on data freshness requirements
- **Monitoring**: Track hit/miss rates

### Application Tuning

- **GOMAXPROCS**: Set based on CPU cores
- **Garbage Collection**: Tune GC if needed
- **Profiling**: Use `go tool pprof` for optimization

## Troubleshooting

### High Latency

1. Check database connection health
2. Review expansion depth metrics
3. Check cache hit rates
4. Review database query performance

### High Error Rate

1. Check database connectivity
2. Review error logs
3. Check rate limiting (may be too restrictive)
4. Review expansion timeouts

### Memory Issues

1. Reduce cache size
2. Check for memory leaks
3. Review expansion depth (deep nesting uses more memory)

### Connection Issues

1. Check database health
2. Review connection pool settings
3. Check network connectivity
4. Review connection pool metrics

## Disaster Recovery

1. **Regular Backups**: Automated daily backups
2. **Backup Testing**: Regularly test restore procedures
3. **Multi-Region**: Consider multi-region deployment for CockroachDB
4. **Failover**: Configure automatic failover
5. **Runbooks**: Document recovery procedures

