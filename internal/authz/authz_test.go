package authz

import (
	"errors"
	"testing"

	"github.com/yxshwanth/zenith/internal/mvcc"
)

func put(db *mvcc.DB, rev mvcc.Revision, t Tuple) {
	_ = db.ApplyPut(rev, t.Key())
}

func TestDirectAllow(t *testing.T) {
	db := mvcc.New()
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob"})
	snap, _ := db.Open(1)
	defer snap.Close()
	e := &Engine{Snap: snap}
	res, err := e.Check("s", "doc", "1", "viewer", "user", "bob")
	if err != nil || res != Allow {
		t.Fatalf("got %v %v", res, err)
	}
}

func TestUsersetUnion(t *testing.T) {
	db := mvcc.New()
	// doc:1#viewer@group:eng#member
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "group", SubjectID: "eng", SubjectRelation: "member"})
	// group:eng#member@user:bob
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "group", ObjectID: "eng", Relation: "member",
		SubjectNamespace: "user", SubjectID: "bob"})
	snap, _ := db.Open(1)
	defer snap.Close()
	e := &Engine{Snap: snap}
	res, err := e.Check("s", "doc", "1", "viewer", "user", "bob")
	if err != nil || res != Allow {
		t.Fatalf("got %v %v", res, err)
	}
}

func TestDiamondSharedSubgraph(t *testing.T) {
	db := mvcc.New()
	// doc viewer via group A and group B; both contain team T; T contains bob
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "group", SubjectID: "A", SubjectRelation: "member"})
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "group", SubjectID: "B", SubjectRelation: "member"})
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "group", ObjectID: "A", Relation: "member",
		SubjectNamespace: "group", SubjectID: "T", SubjectRelation: "member"})
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "group", ObjectID: "B", Relation: "member",
		SubjectNamespace: "group", SubjectID: "T", SubjectRelation: "member"})
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "group", ObjectID: "T", Relation: "member",
		SubjectNamespace: "user", SubjectID: "bob"})
	snap, _ := db.Open(1)
	defer snap.Close()
	e := &Engine{Snap: snap}
	res, err := e.Check("s", "doc", "1", "viewer", "user", "bob")
	if err != nil || res != Allow {
		t.Fatalf("diamond: got %v %v", res, err)
	}
}

func TestBudgetExhaustionIncomplete(t *testing.T) {
	db := mvcc.New()
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "group", SubjectID: "A", SubjectRelation: "member"})
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "group", ObjectID: "A", Relation: "member",
		SubjectNamespace: "user", SubjectID: "bob"})
	snap, _ := db.Open(1)
	defer snap.Close()
	e := &Engine{Snap: snap, Budget: 1}
	res, err := e.Check("s", "doc", "1", "viewer", "user", "bob")
	if res == Allow {
		t.Fatal("budget exhaust must not ALLOW")
	}
	if res != Incomplete && !errors.Is(err, errExhausted) {
		t.Fatalf("want Incomplete/exhausted, got %v %v", res, err)
	}
}

func TestOneRevisionPerCheck(t *testing.T) {
	db := mvcc.New()
	put(db, 1, Tuple{Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob"})
	snap, _ := db.Open(1)
	e := &Engine{Snap: snap}
	e.revSeen = 1
	snap.Close()
	// After close, Contains errors — Check must not ALLOW
	res, err := e.Check("s", "doc", "1", "viewer", "user", "bob")
	if res == Allow {
		t.Fatal("closed snap must not ALLOW")
	}
	_ = err
}
