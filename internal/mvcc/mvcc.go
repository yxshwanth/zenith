// Package mvcc provides in-memory versioned tuple storage with pinned
// snapshots (docs/invariants.md M1–M3). Visibility: latest version of k with revision <= r.
package mvcc

import (
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

// Revision is a committed Raft log index (logical clock).
type Revision uint64

// Key is a length-prefixed canonical tuple key.
type Key string

// EncodeKey builds an unambiguous key from tuple fields.
func EncodeKey(store, ons, oid, rel, sns, sid, srel string) Key {
	parts := []string{store, ons, oid, rel, sns, sid, srel}
	var b []byte
	for _, p := range parts {
		var lenb [4]byte
		binary.BigEndian.PutUint32(lenb[:], uint32(len(p)))
		b = append(b, lenb[:]...)
		b = append(b, p...)
	}
	return Key(b)
}

type version struct {
	rev     Revision
	present bool // false = tombstone
}

// DB is an in-memory MVCC store.
type DB struct {
	mu      sync.Mutex
	chains  map[Key][]version // ascending rev
	floor   Revision          // GC floor H
	pins    map[*snap]struct{}
	applied Revision
}

func New() *DB {
	return &DB{chains: map[Key][]version{}, pins: map[*snap]struct{}{}}
}

// ApplyPut writes a present version at rev (must be > applied).
func (db *DB) ApplyPut(rev Revision, key Key) error {
	return db.apply(rev, key, true)
}

// ApplyDelete writes a tombstone at rev.
func (db *DB) ApplyDelete(rev Revision, key Key) error {
	return db.apply(rev, key, false)
}

// MustApply panics on apply-order violations; those are invariants, not routine errors.
func MustApply(err error) {
	if err != nil {
		panic(err)
	}
}

func (db *DB) apply(rev Revision, key Key, present bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if rev < db.applied {
		return fmt.Errorf("mvcc: apply rev %d < applied %d", rev, db.applied)
	}
	db.applied = rev
	ch := db.chains[key]
	ch = append(ch, version{rev: rev, present: present})
	db.chains[key] = ch
	return nil
}

// SetApplied advances applied watermark without a write (noop/fence).
func (db *DB) SetApplied(rev Revision) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if rev > db.applied {
		db.applied = rev
	}
}

// Snapshot is an immutable view at exactly Revision().
type Snapshot interface {
	Revision() Revision
	Contains(Key) (bool, error)
	Scan(prefix Key, limit int) ([]Key, error)
	Close()
}

type snap struct {
	db  *DB
	rev Revision
}

// Open returns a pinned snapshot at rev. rev must be <= applied and >= floor.
func (db *DB) Open(rev Revision) (Snapshot, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if rev < db.floor {
		return nil, fmt.Errorf("mvcc: revision compacted: %d < floor %d", rev, db.floor)
	}
	if rev > db.applied {
		return nil, fmt.Errorf("mvcc: revision %d not materialized (applied %d)", rev, db.applied)
	}
	s := &snap{db: db, rev: rev}
	db.pins[s] = struct{}{}
	return s, nil
}

func (s *snap) Revision() Revision { return s.rev }

func (s *snap) Contains(key Key) (bool, error) {
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if _, ok := s.db.pins[s]; !ok {
		return false, fmt.Errorf("mvcc: snapshot closed")
	}
	return visible(s.db.chains[key], s.rev), nil
}

func visible(ch []version, r Revision) bool {
	var latest *version
	for i := range ch {
		if ch[i].rev <= r {
			latest = &ch[i]
		}
	}
	if latest == nil {
		return false
	}
	return latest.present
}

func (s *snap) Scan(prefix Key, limit int) ([]Key, error) {
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	if _, ok := s.db.pins[s]; !ok {
		return nil, fmt.Errorf("mvcc: snapshot closed")
	}
	var keys []Key
	for k, ch := range s.db.chains {
		if len(prefix) > 0 && (len(k) < len(prefix) || k[:len(prefix)] != prefix) {
			continue
		}
		if visible(ch, s.rev) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	return keys, nil
}

func (s *snap) Close() {
	s.db.mu.Lock()
	defer s.db.mu.Unlock()
	delete(s.db.pins, s)
}

// GC raises the history floor. Open pins below the new floor cause GC to
// refuse (ponytail: error rather than mutate under a pin).
func (db *DB) GC(newFloor Revision) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	for s := range db.pins {
		if s.rev < newFloor {
			return fmt.Errorf("mvcc: refuse GC to %d: pinned snapshot at %d", newFloor, s.rev)
		}
	}
	db.floor = newFloor
	for k, ch := range db.chains {
		var kept []version
		var newestAtOrBefore *version
		for i := range ch {
			if ch[i].rev > newFloor {
				kept = append(kept, ch[i])
			} else {
				newestAtOrBefore = &ch[i]
			}
		}
		if newestAtOrBefore != nil {
			kept = append([]version{*newestAtOrBefore}, kept...)
		}
		if len(kept) == 0 {
			delete(db.chains, k)
		} else {
			db.chains[k] = kept
		}
	}
	return nil
}

// Applied returns the latest applied revision.
func (db *DB) Applied() Revision {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.applied
}
