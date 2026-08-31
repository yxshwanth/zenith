// Package snapshot publishes and installs durable state checkpoints
// (docs/invariants.md D3). Atomic publish: write temp then rename.
//
// ponytail: JSON blob + rename; in-memory FS for the simulator.
package snapshot

import (
	"encoding/json"
	"fmt"
)

// Meta is the Raft boundary carried with a snapshot.
type Meta struct {
	LastIndex uint64 `json:"lastIndex"`
	LastTerm  uint64 `json:"lastTerm"`
	Epoch     uint64 `json:"epoch"`
}

// State is application bytes (KV + sessions) plus meta.
type State struct {
	Meta     Meta              `json:"meta"`
	KV       map[string]string `json:"kv"`
	Sessions json.RawMessage   `json:"sessions,omitempty"`
}

// Store holds at most one published snapshot generation (ponytail).
type Store struct {
	published []byte
	staging   []byte
}

// BeginWrite stages a snapshot body (crash before Publish leaves old gen).
func (s *Store) BeginWrite(st State) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	s.staging = b
	return nil
}

// Publish atomically promotes staging → published.
func (s *Store) Publish() {
	if s.staging == nil {
		return
	}
	s.published = append([]byte(nil), s.staging...)
	s.staging = nil
}

// CrashDropStaging abandons an in-progress write; the last published snapshot remains.
func (s *Store) CrashDropStaging() { s.staging = nil }

// Load returns the published snapshot, or error if none.
func (s *Store) Load() (State, error) {
	if len(s.published) == 0 {
		return State{}, fmt.Errorf("snapshot: none published")
	}
	var st State
	if err := json.Unmarshal(s.published, &st); err != nil {
		return State{}, err
	}
	if st.KV == nil {
		st.KV = map[string]string{}
	}
	return st, nil
}

// Bytes returns published raw bytes for InstallSnapshot transfer.
func (s *Store) Bytes() []byte { return append([]byte(nil), s.published...) }

// Install replaces published from raw bytes (follower catch-up).
func (s *Store) Install(raw []byte) error {
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return err
	}
	s.published = append([]byte(nil), raw...)
	s.staging = nil
	return nil
}
