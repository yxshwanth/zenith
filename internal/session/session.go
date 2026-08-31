// Package session tracks client request identity for at-most-once apply
// (docs/invariants.md D2). One outstanding mutation per session; reused identity
// with a different digest is an error.
package session

import "fmt"

// Record is the durable per-session outcome retained in applied state.
type Record struct {
	Seq      uint64
	Digest   string
	Result   []byte
	Revision uint64
}

// Table maps session ID → latest record.
type Table map[string]Record

// Decide returns a cached result if this (session, seq, digest) already
// applied, an error if seq/digest conflict, or nil nil if the command
// should execute freshly.
func (t Table) Decide(session string, seq uint64, digest string) (cached []byte, rev uint64, err error) {
	r, ok := t[session]
	if !ok {
		return nil, 0, nil
	}
	if seq < r.Seq {
		return nil, 0, fmt.Errorf("session: stale sequence %d < %d", seq, r.Seq)
	}
	if seq == r.Seq {
		if digest != r.Digest {
			return nil, 0, fmt.Errorf("session: identity reused with different digest")
		}
		return r.Result, r.Revision, nil
	}
	// seq > r.Seq: new request
	return nil, 0, nil
}

// Put stores the outcome after a successful apply.
func (t Table) Put(session string, seq uint64, digest string, result []byte, rev uint64) {
	cp := append([]byte(nil), result...)
	t[session] = Record{Seq: seq, Digest: digest, Result: cp, Revision: rev}
}

// Lookup returns the record for session if seq matches (GetRequestOutcome).
func (t Table) Lookup(session string, seq uint64) (Record, bool) {
	r, ok := t[session]
	if !ok || r.Seq != seq {
		return Record{}, false
	}
	return r, true
}
