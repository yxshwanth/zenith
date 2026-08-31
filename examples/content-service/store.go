// Package contentservice is a minimal content store for the new-enemy demo
// (docs/invariants.md A3). Content bytes are outside Zenith; the token binds versions.
package contentservice

import (
	"fmt"
	"sync"

	"github.com/yxshwanth/zenith/internal/token"
)

// Record is one immutable content version + fence token.
type Record struct {
	Version string
	Object  string
	Bytes   []byte
	Token   string
}

// Store is an in-memory transactional content store (ponytail).
type Store struct {
	mu   sync.Mutex
	data map[string]Record // version -> record
}

func New() *Store { return &Store{data: map[string]Record{}} }

// Publish atomically stores version with its token. Rejects empty token.
func (s *Store) Publish(version, object string, bytes []byte, tok string) error {
	if tok == "" {
		return fmt.Errorf("content: refuse publish without token")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[version]; ok {
		return fmt.Errorf("content: version exists")
	}
	s.data[version] = Record{Version: version, Object: object, Bytes: append([]byte(nil), bytes...), Token: tok}
	return nil
}

// Load returns version and token together (never latest-by-side-channel).
func (s *Store) Load(version string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data[version]
	if !ok {
		return Record{}, fmt.Errorf("content: not found")
	}
	return r, nil
}

// Serve checks the reader may access the loaded record using verify+required revision.
// authz is injected: returns allowed at evaluation revision.
type Authorizer func(requiredRev uint64, reader string, object string) (allowed bool, evalRev uint64, err error)

// Read serves protected bytes only after ALLOW at a revision >= token revision.
func Read(s *Store, codec *token.Codec, storeID, version, reader string, auth Authorizer) ([]byte, error) {
	rec, err := s.Load(version)
	if err != nil {
		return nil, err
	}
	cl, err := codec.Verify(rec.Token, storeID)
	if err != nil {
		return nil, err
	}
	allowed, evalRev, err := auth(cl.Revision, reader, rec.Object)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("content: DENY")
	}
	if evalRev < cl.Revision {
		return nil, fmt.Errorf("content: eval revision %d < required %d", evalRev, cl.Revision)
	}
	return append([]byte(nil), rec.Bytes...), nil
}
