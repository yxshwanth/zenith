package middleware

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func TestTokenBucket(t *testing.T) {
	// Test with 10 RPS and burst of 5
	tb := newTokenBucket(10, 5)

	// Should allow 5 requests immediately (burst)
	for i := 0; i < 5; i++ {
		if !tb.allow() {
			t.Errorf("Request %d should be allowed (burst)", i+1)
		}
	}

	// 6th request should be denied (no tokens left)
	if tb.allow() {
		t.Error("6th request should be denied (no tokens)")
	}

	// Wait a bit for tokens to refill
	time.Sleep(150 * time.Millisecond) // Should refill ~1.5 tokens

	// Should allow at least 1 more request
	if !tb.allow() {
		t.Error("Request after refill should be allowed")
	}
}

func TestRateLimiter_GlobalLimit(t *testing.T) {
	// Create limiter with low global RPS but higher burst
	rl := NewRateLimiter(10, 100, 2) // 10 RPS global, burst of 2

	// First request should pass
	ctx := context.Background()
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	interceptor := rl.UnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Test"}

	// First request should succeed (uses burst)
	_, err := interceptor(ctx, "req1", info, handler)
	if err != nil {
		t.Errorf("First request should succeed: %v", err)
	}

	// Second request should succeed (uses remaining burst)
	_, err = interceptor(ctx, "req2", info, handler)
	if err != nil {
		t.Errorf("Second request should succeed: %v", err)
	}

	// Third request should be rate limited (no tokens left, need to wait for refill)
	_, err = interceptor(ctx, "req3", info, handler)
	if err == nil {
		t.Error("Third request should be rate limited")
	}
	if status, ok := err.(interface{ Code() codes.Code }); ok {
		if status.Code() != codes.ResourceExhausted {
			t.Errorf("Expected ResourceExhausted, got %v", status.Code())
		}
	}

	// Wait for token refill
	time.Sleep(150 * time.Millisecond) // Should refill ~1.5 tokens at 10 RPS

	// Should allow another request after refill
	_, err = interceptor(ctx, "req4", info, handler)
	if err != nil {
		t.Errorf("Request after refill should succeed: %v", err)
	}
}

func TestRateLimiter_PerClientLimit(t *testing.T) {
	// Create limiter with per-client limits (burst of 2 for per-client, very high global to avoid interference)
	rl := NewRateLimiter(10000, 10, 2) // 10000 RPS global with burst 2, 10 RPS per client with burst 2

	ctx1 := peer.NewContext(context.Background(), &peer.Peer{Addr: &mockAddr{addr: "1.2.3.4"}})
	ctx2 := peer.NewContext(context.Background(), &peer.Peer{Addr: &mockAddr{addr: "5.6.7.8"}})

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Test"}
	interceptor := rl.UnaryInterceptor()

	// Client 1: First request should succeed (uses burst)
	_, err := interceptor(ctx1, "req1", info, handler)
	if err != nil {
		t.Errorf("Client 1 first request should succeed: %v", err)
	}

	// Client 1: Second request should succeed (uses remaining burst)
	_, err = interceptor(ctx1, "req2", info, handler)
	if err != nil {
		t.Errorf("Client 1 second request should succeed: %v", err)
	}

	// Client 1: Third request should be rate limited immediately (no tokens left in per-client limiter)
	// Make it immediately without waiting to ensure tokens haven't refilled
	_, err = interceptor(ctx1, "req3", info, handler)
	if err == nil {
		t.Error("Client 1 third request should be rate limited")
	} else {
		if status, ok := err.(interface{ Code() codes.Code }); ok {
			if status.Code() != codes.ResourceExhausted {
				t.Errorf("Expected ResourceExhausted, got %v", status.Code())
			}
		}
	}

	// Wait a bit for tokens to refill
	time.Sleep(200 * time.Millisecond)

	// Client 2: Should still be able to make requests (different client, has its own limiter)
	_, err = interceptor(ctx2, "req1", info, handler)
	if err != nil {
		t.Errorf("Client 2 first request should succeed: %v", err)
	}
}

func TestRateLimiter_ClientIDFromMetadata(t *testing.T) {
	rl := NewRateLimiter(1000, 2, 1)

	// Create context with client ID in metadata
	md := metadata.New(map[string]string{"x-client-id": "client-123"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/test/Test"}
	interceptor := rl.UnaryInterceptor()

	// Should use client ID from metadata
	_, err := interceptor(ctx, "req1", info, handler)
	if err != nil {
		t.Errorf("Request with client ID should succeed: %v", err)
	}
}

type mockAddr struct {
	addr string
}

func (m *mockAddr) Network() string { return "tcp" }
func (m *mockAddr) String() string  { return m.addr }

