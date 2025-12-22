package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/zenith/zenith/internal/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	globalLimiter *tokenBucket
	clientLimiters map[string]*tokenBucket
	mu            sync.RWMutex
	perClientRPS  int
	burstSize     int
}

// tokenBucket implements a simple token bucket algorithm
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

func newTokenBucket(rps int, burst int) *tokenBucket {
	return &tokenBucket{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: float64(rps),
		lastRefill: time.Now(),
	}
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	
	// Refill tokens based on elapsed time
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	// Check if we have enough tokens
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(globalRPS, perClientRPS, burstSize int) *RateLimiter {
	return &RateLimiter{
		globalLimiter:  newTokenBucket(globalRPS, burstSize),
		clientLimiters: make(map[string]*tokenBucket),
		perClientRPS:   perClientRPS,
		burstSize:      burstSize,
	}
}

// getClientID extracts client identifier from context (IP address or client ID)
func (rl *RateLimiter) getClientID(ctx context.Context) string {
	// Try to get client ID from metadata first
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if clientIDs := md.Get("x-client-id"); len(clientIDs) > 0 {
			return clientIDs[0]
		}
	}

	// Fall back to IP address
	if p, ok := peer.FromContext(ctx); ok {
		return p.Addr.String()
	}

	// Default to "unknown"
	return "unknown"
}

// getClientLimiter gets or creates a rate limiter for a specific client
func (rl *RateLimiter) getClientLimiter(clientID string) *tokenBucket {
	rl.mu.RLock()
	limiter, exists := rl.clientLimiters[clientID]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	// Create new limiter for this client
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := rl.clientLimiters[clientID]; exists {
		return limiter
	}

	limiter = newTokenBucket(rl.perClientRPS, rl.burstSize)
	rl.clientLimiters[clientID] = limiter
	return limiter
}

// UnaryInterceptor returns a gRPC unary interceptor for rate limiting
func (rl *RateLimiter) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Check global rate limit
		if !rl.globalLimiter.allow() {
			metrics.RecordRequest("rate_limit", "global_limit_exceeded")
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded: global limit")
		}

		// Check per-client rate limit
		clientID := rl.getClientID(ctx)
		clientLimiter := rl.getClientLimiter(clientID)
		if !clientLimiter.allow() {
			metrics.RecordRequest("rate_limit", "client_limit_exceeded")
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded: client limit")
		}

		// Allow the request
		return handler(ctx, req)
	}
}

// StreamInterceptor returns a gRPC stream interceptor for rate limiting
func (rl *RateLimiter) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		// Check global rate limit
		if !rl.globalLimiter.allow() {
			metrics.RecordRequest("rate_limit", "global_limit_exceeded")
			return status.Errorf(codes.ResourceExhausted, "rate limit exceeded: global limit")
		}

		// Check per-client rate limit
		clientID := rl.getClientID(ctx)
		clientLimiter := rl.getClientLimiter(clientID)
		if !clientLimiter.allow() {
			metrics.RecordRequest("rate_limit", "client_limit_exceeded")
			return status.Errorf(codes.ResourceExhausted, "rate limit exceeded: client limit")
		}

		// Allow the request
		return handler(srv, ss)
	}
}

