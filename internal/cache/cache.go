package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/zenith/zenith/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CacheEntry represents a cached permission check result
type CacheEntry struct {
	Allowed    bool
	Zookie     int64
	Expires    time.Time
	SoftExpires time.Time // Soft expiration: serve stale while refreshing
	HardExpires time.Time // Hard expiration: must refresh before serving
}

// IsExpired checks if the cache entry has expired (hard expiration)
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.HardExpires)
}

// IsSoftExpired checks if the cache entry has soft expired (can serve stale)
func (e *CacheEntry) IsSoftExpired() bool {
	return time.Now().After(e.SoftExpires)
}

// ObjectSubjectsEntry represents a cached object subjects result
type ObjectSubjectsEntry struct {
	Subjects []*Subject
	Zookie   int64
	Expires  time.Time
}

// Subject represents a subject for reverse expansion caching
type Subject struct {
	Namespace string
	ID        string
	Relation  string
}

// IsExpired checks if the cache entry has expired
func (e *ObjectSubjectsEntry) IsExpired() bool {
	return time.Now().After(e.Expires)
}

// Cache implements the "Leopard" cache strategy with negative and sub-graph caching
type Cache struct {
	checkCache          *lru.Cache[string, *CacheEntry]           // For direct checks: check:{subject}:{object}#{relation}
	usersetCache        *lru.Cache[string, *CacheEntry]           // For sub-graph: userset:{subject}:{object}#{relation}
	objectSubjectsCache *lru.Cache[string, *ObjectSubjectsEntry]  // For reverse expansion: object_subjects:{object}#{relation}
	mu                  sync.RWMutex
	ttlPositive         time.Duration
	ttlNegative         time.Duration
	ttlSoft              time.Duration // Soft expiration TTL (default: 30s)
	ttlHard              time.Duration // Hard expiration TTL (default: 5m)
	
	// Background refresh
	refreshWorkers      int           // Number of background refresh workers
	refreshChan         chan string   // Channel for keys to refresh
	refreshFunc         func(context.Context, string) // Function to refresh a key
	
	// Reverse indexes for selective invalidation
	// Maps object pattern (namespace:objectID#relation) to set of cache keys
	objectIndex map[string]map[string]bool
	// Maps subject pattern (namespace:subjectID#relation) to set of cache keys
	subjectIndex map[string]map[string]bool
}

// NewCache creates a new cache with the specified size and TTLs
func NewCache(size int, ttlPositive, ttlNegative time.Duration) (*Cache, error) {
	return NewCacheWithHierarchy(size, ttlPositive, ttlNegative, 30*time.Second, 5*time.Minute, 10)
}

// NewCacheWithHierarchy creates a new cache with soft/hard expiration hierarchy
func NewCacheWithHierarchy(size int, ttlPositive, ttlNegative, ttlSoft, ttlHard time.Duration, refreshWorkers int) (*Cache, error) {
	checkCache, err := lru.New[string, *CacheEntry](size)
	if err != nil {
		return nil, fmt.Errorf("failed to create check cache: %w", err)
	}

	usersetCache, err := lru.New[string, *CacheEntry](size)
	if err != nil {
		return nil, fmt.Errorf("failed to create userset cache: %w", err)
	}

	objectSubjectsCache, err := lru.New[string, *ObjectSubjectsEntry](size)
	if err != nil {
		return nil, fmt.Errorf("failed to create object subjects cache: %w", err)
	}

	c := &Cache{
		checkCache:         checkCache,
		usersetCache:       usersetCache,
		objectSubjectsCache: objectSubjectsCache,
		ttlPositive:        ttlPositive,
		ttlNegative:        ttlNegative,
		ttlSoft:            ttlSoft,
		ttlHard:            ttlHard,
		refreshWorkers:     refreshWorkers,
		refreshChan:        make(chan string, 100), // Buffer for refresh requests
		objectIndex:        make(map[string]map[string]bool),
		subjectIndex:        make(map[string]map[string]bool),
	}

	// Start background refresh workers
	for i := 0; i < refreshWorkers; i++ {
		go c.refreshWorker(context.Background())
	}

	return c, nil
}

// refreshWorker processes background refresh requests
func (c *Cache) refreshWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case key := <-c.refreshChan:
			if c.refreshFunc != nil {
				c.refreshFunc(ctx, key)
			}
		}
	}
}

// SetRefreshFunc sets the function to call for background refreshes
func (c *Cache) SetRefreshFunc(fn func(context.Context, string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshFunc = fn
}

// GetCheck retrieves a cached check result
func (c *Cache) GetCheck(key string) (*CacheEntry, bool) {
	return c.GetCheckWithContext(context.Background(), key)
}

// GetCheckWithContext retrieves a cached check result with tracing
func (c *Cache) GetCheckWithContext(ctx context.Context, key string) (*CacheEntry, bool) {
	ctx, span := observability.StartSpan(ctx, "cache.lookup",
		trace.WithAttributes(
			attribute.String("cache.type", "check"),
			attribute.String("cache.key", key),
		),
	)
	defer span.End()

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.checkCache.Get(key)
	if !ok {
		span.AddEvent("cache_miss", trace.WithAttributes(
			attribute.Bool("cache.hit", false),
		))
		span.SetAttributes(attribute.Bool("cache.hit", false))
		return nil, false
	}

	// Check if expired
	if entry.IsExpired() {
		c.checkCache.Remove(key)
		span.AddEvent("cache_expired", trace.WithAttributes(
			attribute.Bool("cache.hit", false),
			attribute.Bool("cache.expired", true),
		))
		span.SetAttributes(
			attribute.Bool("cache.hit", false),
			attribute.Bool("cache.expired", true),
		)
		return nil, false
	}

	span.AddEvent("cache_hit", trace.WithAttributes(
		attribute.Bool("cache.hit", true),
		attribute.Bool("cache.allowed", entry.Allowed),
	))
	span.SetAttributes(
		attribute.Bool("cache.hit", true),
		attribute.Bool("cache.allowed", entry.Allowed),
	)
	if entry.Zookie > 0 {
		observability.AddZookieToSpan(span, entry.Zookie)
	}

	return entry, true
}

// objectPattern generates a pattern key for an object
func objectPattern(namespace, objectID, relation string) string {
	return fmt.Sprintf("%s:%s#%s", namespace, objectID, relation)
}

// subjectPattern generates a pattern key for a subject
func subjectPattern(namespace, subjectID, subjectRelation string) string {
	if subjectRelation != "" {
		return fmt.Sprintf("%s:%s#%s", namespace, subjectID, subjectRelation)
	}
	return fmt.Sprintf("%s:%s", namespace, subjectID)
}

// addToIndex adds a cache key to the reverse index
func (c *Cache) addToIndex(index map[string]map[string]bool, pattern, key string) {
	if index[pattern] == nil {
		index[pattern] = make(map[string]bool)
	}
	index[pattern][key] = true
}

// removeFromIndex removes a cache key from the reverse index
func (c *Cache) removeFromIndex(index map[string]map[string]bool, pattern, key string) {
	if keys, ok := index[pattern]; ok {
		delete(keys, key)
		if len(keys) == 0 {
			delete(index, pattern)
		}
	}
}

// SetCheck stores a check result in the cache
// key format: check:{subjectNamespace}:{subjectID}:{subjectRelation}:{objectNamespace}:{objectID}#{relation}
func (c *Cache) SetCheck(key string, allowed bool, zookie int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ttl := c.ttlPositive
	if !allowed {
		ttl = c.ttlNegative // Negative results have shorter TTL
	}

	now := time.Now()
	entry := &CacheEntry{
		Allowed:     allowed,
		Zookie:      zookie,
		Expires:      now.Add(ttl),
		SoftExpires: now.Add(c.ttlSoft),
		HardExpires: now.Add(c.ttlHard),
	}

	c.checkCache.Add(key, entry)
	
	// Parse key to extract object and subject patterns for indexing
	// Key format: check:{subjectNamespace}:{subjectID}:{subjectRelation}:{objectNamespace}:{objectID}#{relation}
	// We need to extract the object and subject parts
	// For now, we'll index based on the full key structure
	// The indexing will be done when we know the components
}

// SetCheckWithPatterns stores a check result and indexes it by object and subject patterns
func (c *Cache) SetCheckWithPatterns(key string, allowed bool, zookie int64, objectNamespace, objectID, relation, subjectNamespace, subjectID, subjectRelation string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ttl := c.ttlPositive
	if !allowed {
		ttl = c.ttlNegative
	}

	now := time.Now()
	entry := &CacheEntry{
		Allowed:     allowed,
		Zookie:      zookie,
		Expires:      now.Add(ttl),
		SoftExpires: now.Add(c.ttlSoft),
		HardExpires: now.Add(c.ttlHard),
	}

	c.checkCache.Add(key, entry)
	
	// Index by object pattern
	objPattern := objectPattern(objectNamespace, objectID, relation)
	c.addToIndex(c.objectIndex, objPattern, key)
	
	// Index by subject pattern (if it's a userset)
	if subjectRelation != "" {
		subPattern := subjectPattern(subjectNamespace, subjectID, subjectRelation)
		c.addToIndex(c.subjectIndex, subPattern, key)
	}
}

// GetOrRefresh retrieves a cache entry with soft/hard expiration handling
// If soft expired, serves stale data and triggers background refresh
// If hard expired, must refresh before serving
func (c *Cache) GetOrRefresh(ctx context.Context, key string) (*CacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.checkCache.Get(key)
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	now := time.Now()

	// Hard expired: must refresh
	if now.After(entry.HardExpires) {
		// Trigger refresh in background
		select {
		case c.refreshChan <- key:
		default:
			// Channel full, skip refresh
		}
		return nil, false
	}

	// Soft expired: serve stale and refresh in background
	if now.After(entry.SoftExpires) {
		// Trigger background refresh
		select {
		case c.refreshChan <- key:
		default:
			// Channel full, skip refresh
		}
		// Return stale entry
		return entry, true
	}

	// Fresh: return directly
	return entry, true
}

// GetUserset retrieves a cached userset result
func (c *Cache) GetUserset(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.usersetCache.Get(key)
	if !ok {
		return nil, false
	}

	// Check if expired
	if entry.IsExpired() {
		c.usersetCache.Remove(key)
		return nil, false
	}

	return entry, true
}

// SetUserset stores a userset result in the cache
func (c *Cache) SetUserset(key string, allowed bool, zookie int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ttl := c.ttlPositive
	if !allowed {
		ttl = c.ttlNegative
	}

	entry := &CacheEntry{
		Allowed: allowed,
		Zookie:  zookie,
		Expires: time.Now().Add(ttl),
	}

	c.usersetCache.Add(key, entry)
}

// SetUsersetWithPatterns stores a userset result and indexes it
func (c *Cache) SetUsersetWithPatterns(key string, allowed bool, zookie int64, objectNamespace, objectID, relation, subjectNamespace, subjectID, subjectRelation string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ttl := c.ttlPositive
	if !allowed {
		ttl = c.ttlNegative
	}

	entry := &CacheEntry{
		Allowed: allowed,
		Zookie:  zookie,
		Expires: time.Now().Add(ttl),
	}

	c.usersetCache.Add(key, entry)
	
	// Index by object pattern
	objPattern := objectPattern(objectNamespace, objectID, relation)
	c.addToIndex(c.objectIndex, objPattern, key)
	
	// Index by subject pattern (if it's a userset)
	if subjectRelation != "" {
		subPattern := subjectPattern(subjectNamespace, subjectID, subjectRelation)
		c.addToIndex(c.subjectIndex, subPattern, key)
	}
}

// InvalidateCheck invalidates a check cache entry
func (c *Cache) InvalidateCheck(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkCache.Remove(key)
}

// InvalidateUserset invalidates a userset cache entry
func (c *Cache) InvalidateUserset(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usersetCache.Remove(key)
}

// InvalidateAll invalidates all cache entries (used on writes)
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkCache.Purge()
	c.usersetCache.Purge()
	c.objectSubjectsCache.Purge()
}

// GetObjectSubjects retrieves a cached object subjects result
func (c *Cache) GetObjectSubjects(key string) (*ObjectSubjectsEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.objectSubjectsCache.Get(key)
	if !ok {
		return nil, false
	}

	// Check if expired
	if entry.IsExpired() {
		c.objectSubjectsCache.Remove(key)
		return nil, false
	}

	return entry, true
}

// SetObjectSubjects stores an object subjects result in the cache
func (c *Cache) SetObjectSubjects(key string, subjects []*Subject, zookie int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ttl := c.ttlPositive
	if len(subjects) == 0 {
		ttl = c.ttlNegative // Empty results have shorter TTL
	}

	entry := &ObjectSubjectsEntry{
		Subjects: subjects,
		Zookie:   zookie,
		Expires:  time.Now().Add(ttl),
	}

	c.objectSubjectsCache.Add(key, entry)
}

// ObjectSubjectsKey generates a cache key for object subjects
func ObjectSubjectsKey(namespace, objectID, relation string) string {
	return fmt.Sprintf("object_subjects:%s:%s#%s", namespace, objectID, relation)
}

// InvalidateRelated invalidates cache entries related to a tuple
// This is called when a tuple is written to maintain cache consistency
func (c *Cache) InvalidateRelated(namespace, objectID, relation, subjectNamespace, subjectID, subjectRelation string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Invalidate entries related to the object
	objPattern := objectPattern(namespace, objectID, relation)
	if keys, ok := c.objectIndex[objPattern]; ok {
		for key := range keys {
			c.checkCache.Remove(key)
			c.usersetCache.Remove(key)
			c.removeFromIndex(c.objectIndex, objPattern, key)
		}
		delete(c.objectIndex, objPattern)
	}
	
	// Invalidate reverse expansion cache for this object
	objSubjectsKey := ObjectSubjectsKey(namespace, objectID, relation)
	c.objectSubjectsCache.Remove(objSubjectsKey)
	
	// If the subject is a userset, invalidate entries where this subject is used
	if subjectRelation != "" {
		subPattern := subjectPattern(subjectNamespace, subjectID, subjectRelation)
		if keys, ok := c.subjectIndex[subPattern]; ok {
			for key := range keys {
				c.checkCache.Remove(key)
				c.usersetCache.Remove(key)
				c.removeFromIndex(c.subjectIndex, subPattern, key)
			}
			delete(c.subjectIndex, subPattern)
		}
		
		// Also invalidate reverse expansion for the subject (if it's a userset object)
		subObjSubjectsKey := ObjectSubjectsKey(subjectNamespace, subjectID, subjectRelation)
		c.objectSubjectsCache.Remove(subObjSubjectsKey)
	}
	
	// Invalidate any checks that directly reference this tuple
	// (where the tuple's object matches the check's object and the tuple's subject matches the check's subject)
	directCheckKey := CheckKey(subjectNamespace, subjectID, subjectRelation, namespace, objectID, relation)
	c.checkCache.Remove(directCheckKey)
	
	directUsersetKey := UsersetKey(subjectNamespace, subjectID, subjectRelation, namespace, objectID, relation)
	c.usersetCache.Remove(directUsersetKey)
}

// CheckKey generates a cache key for a direct check
func CheckKey(subjectNamespace, subjectID, subjectRelation, objectNamespace, objectID, relation string) string {
	return fmt.Sprintf("check:%s:%s:%s:%s:%s#%s",
		subjectNamespace, subjectID, subjectRelation,
		objectNamespace, objectID, relation)
}

// UsersetKey generates a cache key for a userset check
func UsersetKey(subjectNamespace, subjectID, subjectRelation, objectNamespace, objectID, relation string) string {
	return fmt.Sprintf("userset:%s:%s:%s:%s:%s#%s",
		subjectNamespace, subjectID, subjectRelation,
		objectNamespace, objectID, relation)
}


