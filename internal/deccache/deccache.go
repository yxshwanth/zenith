// Package deccache is a revision-keyed Check decision cache (C1).
// Hits are valid only when the cached revision equals the selected snapshot.
package deccache

import "sync"

// Key identifies one Check evaluation.
type Key struct {
	Store, ONS, OID, Rel, SNS, SID string
	Revision                       uint64
}

// Entry is a cached ALLOW/DENY (never Incomplete).
type Entry struct {
	Allowed bool
}

// Cache is an in-process map (ponytail: global lock; shard if hot).
type Cache struct {
	mu   sync.Mutex
	data map[Key]Entry
}

func New() *Cache { return &Cache{data: map[Key]Entry{}} }

// Get returns a hit only for the exact revision.
func (c *Cache) Get(k Key) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[k]
	return e, ok
}

// Put stores an ALLOW/DENY at revision. Incomplete must not be cached.
func (c *Cache) Put(k Key, allowed bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[k] = Entry{Allowed: allowed}
}
