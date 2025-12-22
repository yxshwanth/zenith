package cache

import (
	"testing"
	"time"
)

func TestCache_NegativeCaching(t *testing.T) {
	cache, err := NewCache(100, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	key := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")

	// Set negative result
	cache.SetCheck(key, false, 1000)

	// Retrieve from cache
	entry, ok := cache.GetCheck(key)
	if !ok {
		t.Error("Expected cache hit")
	}
	if entry.Allowed {
		t.Error("Expected allowed=false")
	}
	if entry.Zookie != 1000 {
		t.Errorf("Expected zookie=1000, got %d", entry.Zookie)
	}
}

func TestCache_SubGraphCaching(t *testing.T) {
	cache, err := NewCache(100, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	key := UsersetKey("user", "alice", "", "group", "eng", "member")

	// Set userset result
	cache.SetUserset(key, true, 2000)

	// Retrieve from cache
	entry, ok := cache.GetUserset(key)
	if !ok {
		t.Error("Expected cache hit")
	}
	if !entry.Allowed {
		t.Error("Expected allowed=true")
	}
	if entry.Zookie != 2000 {
		t.Errorf("Expected zookie=2000, got %d", entry.Zookie)
	}
}

func TestCache_Expiration(t *testing.T) {
	cache, err := NewCache(100, 100*time.Millisecond, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	key := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")

	// Set entry
	cache.SetCheck(key, true, 1000)

	// Should be in cache
	_, ok := cache.GetCheck(key)
	if !ok {
		t.Error("Expected cache hit before expiration")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, ok = cache.GetCheck(key)
	if ok {
		t.Error("Expected cache miss after expiration")
	}
}

func TestCache_Invalidation(t *testing.T) {
	cache, err := NewCache(100, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	key := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")
	cache.SetCheck(key, true, 1000)

	// Invalidate
	cache.InvalidateCheck(key)

	// Should be gone
	_, ok := cache.GetCheck(key)
	if ok {
		t.Error("Expected cache miss after invalidation")
	}
}

func TestCache_InvalidateAll(t *testing.T) {
	cache, err := NewCache(100, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	key1 := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")
	key2 := UsersetKey("user", "bob", "", "group", "eng", "member")

	cache.SetCheck(key1, true, 1000)
	cache.SetUserset(key2, true, 2000)

	// Invalidate all
	cache.InvalidateAll()

	// Both should be gone
	_, ok := cache.GetCheck(key1)
	if ok {
		t.Error("Expected cache miss after InvalidateAll")
	}
	_, ok = cache.GetUserset(key2)
	if ok {
		t.Error("Expected cache miss after InvalidateAll")
	}
}

func TestCache_SelectiveInvalidation_ObjectPattern(t *testing.T) {
	cache, err := NewCache(100, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Set up multiple cache entries for the same object but different subjects
	key1 := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")
	key2 := CheckKey("user", "bob", "", "doc", "doc_1", "viewer")
	key3 := CheckKey("user", "charlie", "", "doc", "doc_1", "editor")
	key4 := CheckKey("user", "alice", "", "doc", "doc_2", "viewer") // Different object

	// Set entries with patterns for indexing
	cache.SetCheckWithPatterns(key1, true, 1000, "doc", "doc_1", "viewer", "user", "alice", "")
	cache.SetCheckWithPatterns(key2, true, 2000, "doc", "doc_1", "viewer", "user", "bob", "")
	cache.SetCheckWithPatterns(key3, true, 3000, "doc", "doc_1", "editor", "user", "charlie", "")
	cache.SetCheckWithPatterns(key4, true, 4000, "doc", "doc_2", "viewer", "user", "alice", "")

	// Verify all entries are cached
	if _, ok := cache.GetCheck(key1); !ok {
		t.Error("Expected key1 to be cached")
	}
	if _, ok := cache.GetCheck(key2); !ok {
		t.Error("Expected key2 to be cached")
	}
	if _, ok := cache.GetCheck(key3); !ok {
		t.Error("Expected key3 to be cached")
	}
	if _, ok := cache.GetCheck(key4); !ok {
		t.Error("Expected key4 to be cached")
	}

	// Invalidate entries related to doc:doc_1#viewer
	// This should remove key1 and key2, but not key3 (different relation) or key4 (different object)
	cache.InvalidateRelated("doc", "doc_1", "viewer", "user", "alice", "")

	// key1 and key2 should be invalidated (same object and relation)
	if _, ok := cache.GetCheck(key1); ok {
		t.Error("Expected key1 to be invalidated")
	}
	if _, ok := cache.GetCheck(key2); ok {
		t.Error("Expected key2 to be invalidated")
	}

	// key3 should remain (different relation)
	if _, ok := cache.GetCheck(key3); !ok {
		t.Error("Expected key3 to remain cached (different relation)")
	}

	// key4 should remain (different object)
	if _, ok := cache.GetCheck(key4); !ok {
		t.Error("Expected key4 to remain cached (different object)")
	}
}

func TestCache_SelectiveInvalidation_UsersetSubject(t *testing.T) {
	cache, err := NewCache(100, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Set up cache entries where a userset is used as subject
	// doc_1#viewer -> group:eng#member (userset)
	key1 := CheckKey("group", "eng", "member", "doc", "doc_1", "viewer")
	cache.SetCheckWithPatterns(key1, true, 1000, "doc", "doc_1", "viewer", "group", "eng", "member")

	// Set up entries that check if users have the userset relation
	// user:alice -> group:eng#member
	key2 := CheckKey("user", "alice", "", "group", "eng", "member")
	cache.SetCheckWithPatterns(key2, true, 2000, "group", "eng", "member", "user", "alice", "")

	// Set up unrelated entry
	key3 := CheckKey("user", "bob", "", "doc", "doc_2", "viewer")
	cache.SetCheckWithPatterns(key3, true, 3000, "doc", "doc_2", "viewer", "user", "bob", "")

	// Verify all entries are cached
	if _, ok := cache.GetCheck(key1); !ok {
		t.Error("Expected key1 to be cached")
	}
	if _, ok := cache.GetCheck(key2); !ok {
		t.Error("Expected key2 to be cached")
	}
	if _, ok := cache.GetCheck(key3); !ok {
		t.Error("Expected key3 to be cached")
	}

	// Invalidate entries related to group:eng#member (the userset)
	// This should invalidate key2 (which checks the userset relation)
	cache.InvalidateRelated("group", "eng", "member", "user", "alice", "")

	// key2 should be invalidated (directly related to the userset)
	if _, ok := cache.GetCheck(key2); ok {
		t.Error("Expected key2 to be invalidated (related to userset)")
	}

	// key1 might be invalidated if it's indexed by subject pattern
	// key3 should remain (unrelated)
	if _, ok := cache.GetCheck(key3); !ok {
		t.Error("Expected key3 to remain cached (unrelated)")
	}
}

func TestCache_SelectiveInvalidation_ReverseExpansion(t *testing.T) {
	cache, err := NewCache(100, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Set up reverse expansion cache entry
	objKey := ObjectSubjectsKey("doc", "doc_1", "viewer")
	subjects := []*Subject{
		{Namespace: "user", ID: "alice", Relation: ""},
		{Namespace: "user", ID: "bob", Relation: ""},
	}
	cache.SetObjectSubjects(objKey, subjects, 1000)

	// Set up another reverse expansion cache for different object
	objKey2 := ObjectSubjectsKey("doc", "doc_2", "viewer")
	subjects2 := []*Subject{
		{Namespace: "user", ID: "charlie", Relation: ""},
	}
	cache.SetObjectSubjects(objKey2, subjects2, 2000)

	// Verify both are cached
	if _, ok := cache.GetObjectSubjects(objKey); !ok {
		t.Error("Expected objKey to be cached")
	}
	if _, ok := cache.GetObjectSubjects(objKey2); !ok {
		t.Error("Expected objKey2 to be cached")
	}

	// Invalidate entries related to doc:doc_1#viewer
	cache.InvalidateRelated("doc", "doc_1", "viewer", "user", "alice", "")

	// objKey should be invalidated
	if _, ok := cache.GetObjectSubjects(objKey); ok {
		t.Error("Expected objKey to be invalidated")
	}

	// objKey2 should remain (different object)
	if _, ok := cache.GetObjectSubjects(objKey2); !ok {
		t.Error("Expected objKey2 to remain cached (different object)")
	}
}

func TestCache_SelectiveInvalidation_UnrelatedEntriesPreserved(t *testing.T) {
	cache, err := NewCache(100, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Set up multiple unrelated cache entries
	key1 := CheckKey("user", "alice", "", "doc", "doc_1", "viewer")
	key2 := CheckKey("user", "bob", "", "folder", "folder_1", "editor")
	key3 := CheckKey("user", "charlie", "", "repo", "repo_1", "admin")

	cache.SetCheckWithPatterns(key1, true, 1000, "doc", "doc_1", "viewer", "user", "alice", "")
	cache.SetCheckWithPatterns(key2, true, 2000, "folder", "folder_1", "editor", "user", "bob", "")
	cache.SetCheckWithPatterns(key3, true, 3000, "repo", "repo_1", "admin", "user", "charlie", "")

	// Invalidate only doc:doc_1#viewer
	cache.InvalidateRelated("doc", "doc_1", "viewer", "user", "alice", "")

	// key1 should be invalidated
	if _, ok := cache.GetCheck(key1); ok {
		t.Error("Expected key1 to be invalidated")
	}

	// key2 and key3 should remain
	if _, ok := cache.GetCheck(key2); !ok {
		t.Error("Expected key2 to remain cached")
	}
	if _, ok := cache.GetCheck(key3); !ok {
		t.Error("Expected key3 to remain cached")
	}
}

