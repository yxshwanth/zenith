package authz_test

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/authz"
	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/mvcc"
)

// TestAuthzMatchesGraphOracle is the Phase 3 gate: independent graph oracle
// agrees with authz on small fixtures (A1).
func TestAuthzMatchesGraphOracle(t *testing.T) {
	edges := []checker.GraphEdge{
		{ObjectNS: "doc", ObjectID: "1", Relation: "viewer", SubjectNS: "group", SubjectID: "A", SubjectRel: "member"},
		{ObjectNS: "group", ObjectID: "A", Relation: "member", SubjectNS: "user", SubjectID: "bob"},
		{ObjectNS: "doc", ObjectID: "1", Relation: "viewer", SubjectNS: "user", SubjectID: "alice"},
	}
	db := mvcc.New()
	for _, e := range edges {
		t := authz.Tuple{
			Store: "s", ObjectNamespace: e.ObjectNS, ObjectID: e.ObjectID, Relation: e.Relation,
			SubjectNamespace: e.SubjectNS, SubjectID: e.SubjectID, SubjectRelation: e.SubjectRel,
		}
		_ = db.ApplyPut(1, t.Key())
	}
	snap, _ := db.Open(1)
	defer snap.Close()
	eng := &authz.Engine{Snap: snap}

	cases := []struct{ ons, oid, rel, sns, sid string }{
		{"doc", "1", "viewer", "user", "bob"},
		{"doc", "1", "viewer", "user", "alice"},
		{"doc", "1", "viewer", "user", "eve"},
	}
	for _, c := range cases {
		want := checker.GraphAllows(edges, c.ons, c.oid, c.rel, c.sns, c.sid)
		res, err := eng.Check("s", c.ons, c.oid, c.rel, c.sns, c.sid)
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		got := res == authz.Allow
		if got != want {
			t.Fatalf("%+v: authz=%v oracle=%v", c, got, want)
		}
	}
}

func TestMVCCMatchesVersionOracle(t *testing.T) {
	ops := []checker.VersionOp{
		{Rev: 1, Key: "bob", Present: true},
		{Rev: 2, Key: "alice", Present: true},
		{Rev: 3, Key: "bob", Present: false},
	}
	db := mvcc.New()
	kb := mvcc.EncodeKey("s", "doc", "1", "viewer", "user", "bob", "")
	ka := mvcc.EncodeKey("s", "doc", "1", "viewer", "user", "alice", "")
	_ = db.ApplyPut(1, kb)
	_ = db.ApplyPut(2, ka)
	_ = db.ApplyDelete(3, kb)

	for _, r := range []mvcc.Revision{1, 2, 3} {
		snap, err := db.Open(r)
		if err != nil {
			t.Fatal(err)
		}
		ob, _ := snap.Contains(kb)
		oa, _ := snap.Contains(ka)
		snap.Close()
		if ob != checker.VisibleAt(ops, "bob", uint64(r)) {
			t.Fatalf("bob at %d: mvcc=%v oracle=%v", r, ob, checker.VisibleAt(ops, "bob", uint64(r)))
		}
		if oa != checker.VisibleAt(ops, "alice", uint64(r)) {
			t.Fatalf("alice at %d mismatch", r)
		}
	}
}
