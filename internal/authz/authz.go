// Package authz evaluates ReBAC at one pinned MVCC snapshot (docs/invariants.md A1–A4).
package authz

import (
	"fmt"

	"github.com/yxshwanth/zenith/internal/mvcc"
)

// Tuple is the vocabulary used by Check.
type Tuple struct {
	Store            string
	ObjectNamespace  string
	ObjectID         string
	Relation         string
	SubjectNamespace string
	SubjectID        string
	SubjectRelation  string // empty = direct user
}

func (t Tuple) Key() mvcc.Key {
	return mvcc.EncodeKey(t.Store, t.ObjectNamespace, t.ObjectID, t.Relation,
		t.SubjectNamespace, t.SubjectID, t.SubjectRelation)
}

// Result of a Check.
type Result int

const (
	Allow Result = iota
	Deny
	Incomplete // work budget / typed incomplete — never treat as ALLOW
)

func (r Result) String() string {
	switch r {
	case Allow:
		return "ALLOW"
	case Deny:
		return "DENY"
	default:
		return "INCOMPLETE"
	}
}

// RelRewrite is Phase-B relation semantics (intersection / exclusion / TTU).
type RelRewrite struct {
	Kind     string // "", "intersection", "exclusion", "ttu"
	Children []string
	Tupleset string
	Computed string
}

// Schema maps namespace → relation → rewrite.
type Schema map[string]map[string]RelRewrite

// Engine evaluates against a single snapshot handle for the whole Check.
type Engine struct {
	Snap    mvcc.Snapshot
	Budget  int // max Contains calls; 0 = unlimited
	Schema  Schema
	used    int
	revSeen mvcc.Revision
}

// Check reports whether subject has relation on object.
func (e *Engine) Check(store, ons, oid, rel, sns, sid string) (Result, error) {
	if e.Snap == nil {
		return Incomplete, fmt.Errorf("authz: nil snapshot")
	}
	e.revSeen = e.Snap.Revision()
	return e.check(store, ons, oid, rel, sns, sid, nil, 0)
}

func (e *Engine) check(store, ons, oid, rel, sns, sid string, path map[string]bool, exclDepth int) (Result, error) {
	if e.Snap.Revision() != e.revSeen {
		return Incomplete, fmt.Errorf("authz: mixed revision %d vs %d", e.Snap.Revision(), e.revSeen)
	}
	id := store + "|" + ons + "|" + oid + "|" + rel + "|" + sns + "|" + sid
	if path[id] {
		return Deny, nil
	}
	if path == nil {
		path = map[string]bool{}
	}
	path[id] = true
	defer delete(path, id)

	if rw, ok := e.schemaRel(ons, rel); ok {
		switch rw.Kind {
		case "intersection":
			return e.checkIntersection(store, ons, oid, sns, sid, path, exclDepth, rw)
		case "exclusion":
			return e.checkExclusion(store, ons, oid, sns, sid, path, exclDepth, rw)
		case "ttu":
			return e.checkTTU(store, ons, oid, sns, sid, path, exclDepth, rw)
		}
	}

	direct := Tuple{Store: store, ObjectNamespace: ons, ObjectID: oid, Relation: rel,
		SubjectNamespace: sns, SubjectID: sid, SubjectRelation: ""}
	ok, err := e.contains(direct.Key())
	if err != nil {
		return Incomplete, err
	}
	if ok {
		return Allow, nil
	}

	keys, err := e.Snap.Scan("", 0)
	if err != nil {
		return Incomplete, err
	}
	for _, k := range keys {
		oid2, rel2, sns2, sid2, srel, ok := decodeUserset(k, store)
		if !ok {
			continue
		}
		if oid2 != oid || rel2 != rel || srel == "" {
			continue
		}
		okp, err := e.contains(k)
		if err != nil {
			return Incomplete, err
		}
		if !okp {
			continue
		}
		res, err := e.check(store, sns2, sid2, srel, sns, sid, path, exclDepth)
		if err != nil {
			return Incomplete, err
		}
		if res == Allow {
			return Allow, nil
		}
		if res == Incomplete {
			return Incomplete, nil
		}
	}
	return Deny, nil
}

func (e *Engine) checkIntersection(store, ons, oid, sns, sid string, path map[string]bool, exclDepth int, rw RelRewrite) (Result, error) {
	for _, child := range rw.Children {
		res, err := e.check(store, ons, oid, child, sns, sid, path, exclDepth)
		if err != nil || res == Incomplete {
			return Incomplete, err
		}
		if res != Allow {
			return Deny, nil
		}
	}
	if len(rw.Children) == 0 {
		return Deny, nil
	}
	return Allow, nil
}

func (e *Engine) checkExclusion(store, ons, oid, sns, sid string, path map[string]bool, exclDepth int, rw RelRewrite) (Result, error) {
	if exclDepth > 0 {
		return Deny, nil // stratified: no nested exclusion
	}
	if len(rw.Children) < 2 {
		return Deny, nil
	}
	base, err := e.check(store, ons, oid, rw.Children[0], sns, sid, path, exclDepth+1)
	if err != nil || base == Incomplete {
		return Incomplete, err
	}
	if base != Allow {
		return Deny, nil
	}
	sub, err := e.check(store, ons, oid, rw.Children[1], sns, sid, path, exclDepth+1)
	if err != nil || sub == Incomplete {
		return Incomplete, err
	}
	if sub == Allow {
		return Deny, nil
	}
	return Allow, nil
}

func (e *Engine) checkTTU(store, ons, oid, sns, sid string, path map[string]bool, exclDepth int, rw RelRewrite) (Result, error) {
	keys, err := e.Snap.Scan("", 0)
	if err != nil {
		return Incomplete, err
	}
	for _, k := range keys {
		parts, ok := decodeKey(k)
		if !ok || len(parts) != 7 || parts[0] != store {
			continue
		}
		if parts[1] != ons || parts[2] != oid || parts[3] != rw.Tupleset || parts[6] != "" {
			continue
		}
		okp, err := e.contains(k)
		if err != nil {
			return Incomplete, err
		}
		if !okp {
			continue
		}
		res, err := e.check(store, parts[4], parts[5], rw.Computed, sns, sid, path, exclDepth)
		if err != nil || res == Incomplete {
			return Incomplete, err
		}
		if res == Allow {
			return Allow, nil
		}
	}
	return Deny, nil
}

func (e *Engine) schemaRel(ns, rel string) (RelRewrite, bool) {
	if e.Schema == nil {
		return RelRewrite{}, false
	}
	m, ok := e.Schema[ns]
	if !ok {
		return RelRewrite{}, false
	}
	rw, ok := m[rel]
	return rw, ok
}

// ListSubjects returns direct subjects for object#relation at the pinned snap.
func (e *Engine) ListSubjects(store, ons, oid, rel string, limit int) ([]Tuple, error) {
	if e.Snap == nil {
		return nil, fmt.Errorf("authz: nil snapshot")
	}
	if limit <= 0 {
		limit = 256
	}
	e.revSeen = e.Snap.Revision()
	if rw, ok := e.schemaRel(ons, rel); ok && rw.Kind == "exclusion" {
		// Complete-or-error: expand base and subtract; never emit speculative members.
		if len(rw.Children) < 2 {
			return nil, nil
		}
		base, err := e.ListSubjects(store, ons, oid, rw.Children[0], limit)
		if err != nil {
			return nil, err
		}
		sub, err := e.ListSubjects(store, ons, oid, rw.Children[1], limit)
		if err != nil {
			return nil, err
		}
		drop := map[string]bool{}
		for _, s := range sub {
			drop[s.SubjectNamespace+"|"+s.SubjectID] = true
		}
		var out []Tuple
		for _, s := range base {
			if drop[s.SubjectNamespace+"|"+s.SubjectID] {
				continue
			}
			out = append(out, s)
		}
		return out, nil
	}
	keys, err := e.Snap.Scan("", 0)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Tuple
	var walk func(ons, oid, rel string, path map[string]bool) error
	walk = func(ons, oid, rel string, path map[string]bool) error {
		id := ons + "|" + oid + "|" + rel
		if path[id] {
			return nil
		}
		path[id] = true
		for _, k := range keys {
			parts, ok := decodeKey(k)
			if !ok || len(parts) != 7 || parts[0] != store {
				continue
			}
			if parts[1] != ons || parts[2] != oid || parts[3] != rel {
				continue
			}
			okp, err := e.contains(k)
			if err != nil {
				return err
			}
			if !okp {
				continue
			}
			if parts[6] == "" {
				key := parts[4] + "|" + parts[5]
				if seen[key] {
					continue
				}
				if len(out) >= limit {
					return errExhausted
				}
				seen[key] = true
				out = append(out, Tuple{Store: store, ObjectNamespace: ons, ObjectID: oid, Relation: rel,
					SubjectNamespace: parts[4], SubjectID: parts[5]})
				continue
			}
			if err := walk(parts[4], parts[5], parts[6], path); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(ons, oid, rel, map[string]bool{}); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeKey(k mvcc.Key) ([]string, bool) {
	b := []byte(k)
	var parts []string
	for len(b) >= 4 {
		n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
		b = b[4:]
		if n < 0 || n > len(b) {
			return nil, false
		}
		parts = append(parts, string(b[:n]))
		b = b[n:]
	}
	if len(b) != 0 {
		return nil, false
	}
	return parts, true
}

func decodeUserset(k mvcc.Key, store string) (oid, rel, sns, sid, srel string, ok bool) {
	parts, ok := decodeKey(k)
	if !ok || len(parts) != 7 || parts[0] != store {
		return "", "", "", "", "", false
	}
	return parts[2], parts[3], parts[4], parts[5], parts[6], true
}

func (e *Engine) contains(k mvcc.Key) (bool, error) {
	if e.Budget > 0 {
		e.used++
		if e.used > e.Budget {
			return false, errExhausted
		}
	}
	if e.Snap.Revision() != e.revSeen {
		return false, fmt.Errorf("authz: mixed revision")
	}
	return e.Snap.Contains(k)
}

var errExhausted = fmt.Errorf("authz: resource exhausted")
