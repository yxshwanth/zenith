package cache

import (
	"testing"
	"time"
)

// BenchmarkCache_GetSet benchmarks cache get/set operations
func BenchmarkCache_GetSet(b *testing.B) {
	cache, _ := NewCache(10000, 30*time.Second, 5*time.Second)
	key := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetCheck(key, true, 1000)
		_, _ = cache.GetCheck(key)
	}
}

// BenchmarkCache_GetSetWithPatterns benchmarks cache with pattern indexing
func BenchmarkCache_GetSetWithPatterns(b *testing.B) {
	cache, _ := NewCache(10000, 30*time.Second, 5*time.Second)
	key := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetCheckWithPatterns(key, true, 1000,
			"doc", "doc_1", "viewer",
			"user", "alice", "")
		_, _ = cache.GetCheck(key)
	}
}

// BenchmarkCache_InvalidateRelated benchmarks selective invalidation
func BenchmarkCache_InvalidateRelated(b *testing.B) {
	cache, _ := NewCache(10000, 30*time.Second, 5*time.Second)
	
	// Pre-populate cache
	for i := 0; i < 100; i++ {
		key := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")
		cache.SetCheckWithPatterns(key, true, 1000,
			"doc", "doc_1", "viewer",
			"user", "alice", "")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.InvalidateRelated("doc", "doc_1", "viewer", "user", "alice", "")
	}
}

// BenchmarkCheckKey benchmarks cache key generation
func BenchmarkCheckKey(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CheckKey("user", "alice", "", "doc", "doc_1", "viewer")
	}
}

