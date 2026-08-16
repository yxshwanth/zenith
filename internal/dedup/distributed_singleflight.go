package dedup

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// DistributedSingleflight provides distributed request deduplication using Redis
// Falls back to local singleflight if Redis is unavailable
type DistributedSingleflight struct {
	redis      *redis.Client
	local      singleflight.Group
	keyPrefix  string
	lockTTL    time.Duration
	resultTTL  time.Duration
	fallback   bool // Whether to fallback to local singleflight
}

// Config holds configuration for distributed singleflight
type Config struct {
	Redis      *redis.Client
	KeyPrefix  string        // Prefix for Redis keys (default: "zenith:dedup:")
	LockTTL    time.Duration // TTL for lock keys (default: 5s)
	ResultTTL  time.Duration // TTL for result cache (default: 10s)
}

// DefaultConfig returns a default configuration
func DefaultConfig(redisClient *redis.Client) Config {
	return Config{
		Redis:     redisClient,
		KeyPrefix: "zenith:dedup:",
		LockTTL:   5 * time.Second,
		ResultTTL: 10 * time.Second,
	}
}

// NewDistributedSingleflight creates a new distributed singleflight instance
func NewDistributedSingleflight(config Config) *DistributedSingleflight {
	return &DistributedSingleflight{
		redis:     config.Redis,
		local:     singleflight.Group{},
		keyPrefix: config.KeyPrefix,
		lockTTL:   config.LockTTL,
		resultTTL: config.ResultTTL,
		fallback:  config.Redis == nil,
	}
}

// Result represents a cached result
type Result struct {
	Value interface{}
	Error error
}

// Do executes the function with distributed deduplication
// If Redis is available, uses distributed locking
// Otherwise, falls back to local singleflight
func (dsf *DistributedSingleflight) Do(ctx context.Context, key string, fn func() (interface{}, error)) (interface{}, error, bool) {
	// Fallback to local singleflight if Redis unavailable
	if dsf.fallback || dsf.redis == nil {
		return dsf.local.Do(key, fn)
	}

	lockKey := dsf.keyPrefix + "lock:" + key
	resultKey := dsf.keyPrefix + "result:" + key

	// Try to acquire distributed lock
	acquired, err := dsf.redis.SetNX(ctx, lockKey, "1", dsf.lockTTL).Result()
	if err != nil {
		// Redis error, fallback to local
		return dsf.local.Do(key, fn)
	}

	if acquired {
		// First instance: do work, cache result
		result, err := fn()

		// Cache result in Redis
		resultData, marshalErr := json.Marshal(Result{Value: result, Error: err})
		if marshalErr == nil {
			dsf.redis.Set(ctx, resultKey, resultData, dsf.resultTTL)
		}

		// Release lock
		dsf.redis.Del(ctx, lockKey)

		return result, err, false
	}

	// Other instances: wait for result from first instance
	// Poll for result with timeout
	timeout := time.After(dsf.lockTTL + 1*time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, fallback to local
			return dsf.local.Do(key, fn)
		case <-timeout:
			// Timeout waiting for result, fallback to local
			return dsf.local.Do(key, fn)
		case <-ticker.C:
			// Check for result
			resultData, err := dsf.redis.Get(ctx, resultKey).Result()
			if err == redis.Nil {
				// Result not ready yet, continue polling
				continue
			}
			if err != nil {
				// Redis error, fallback to local
				return dsf.local.Do(key, fn)
			}

			// Unmarshal result
			var result Result
			if err := json.Unmarshal([]byte(resultData), &result); err != nil {
				// Invalid result, fallback to local
				return dsf.local.Do(key, fn)
			}

			return result.Value, result.Error, true
		}
	}
}

// DoChan executes the function and returns a channel for the result
func (dsf *DistributedSingleflight) DoChan(ctx context.Context, key string, fn func() (interface{}, error)) <-chan singleflight.Result {
	ch := make(chan singleflight.Result, 1)

	go func() {
		val, err, shared := dsf.Do(ctx, key, fn)
		ch <- singleflight.Result{Val: val, Err: err, Shared: shared}
		close(ch)
	}()

	return ch
}

// Forget tells the singleflight to forget about a key
func (dsf *DistributedSingleflight) Forget(key string) {
	dsf.local.Forget(key)
	if dsf.redis != nil && !dsf.fallback {
		ctx := context.Background()
		lockKey := dsf.keyPrefix + "lock:" + key
		resultKey := dsf.keyPrefix + "result:" + key
		dsf.redis.Del(ctx, lockKey, resultKey)
	}
}

