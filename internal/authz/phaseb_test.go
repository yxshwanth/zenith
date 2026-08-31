package authz_test

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/authz"
	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/mvcc"
)

func TestPhaseBMatchesOracle(t *testing.T) {
	edges := []checker.GraphEdge{
		{ObjectNS: "doc", ObjectID: "1", Relation: "owner", SubjectNS: "user", SubjectID: "bob"},
		{ObjectNS: "doc", ObjectID: "1", Relation: "banned", SubjectNS: "user", SubjectID: "bob"},
		{ObjectNS: "doc", ObjectID: "1", Relation: "owner", SubjectNS: "user", SubjectID: "alice"},
		{ObjectNS: "doc", ObjectID: "1", Relation: "parent", SubjectNS: "folder", SubjectID: "f1"},
		{ObjectNS: "folder", ObjectID: "f1", Relation: "viewer", SubjectNS: "user", SubjectID: "carol"},
		{ObjectNS: "doc", ObjectID: "1", Relation: "editor", SubjectNS: "user", SubjectID: "dave"},
		{ObjectNS: "doc", ObjectID: "1", Relation: "viewer", SubjectNS: "user", SubjectID: "dave"},
	}
	schema := checker.Schema{
		"doc": {
			"can_view":          {Kind: "exclusion", Children: []string{"owner", "banned"}},
			"via_parent":        {Kind: "ttu", Tupleset: "parent", Computed: "viewer"},
			"editor_and_viewer": {Kind: "intersection", Children: []string{"editor", "viewer"}},
		},
	}
	db := mvcc.New()
	for i, e := range edges {
		tu := authz.Tuple{
			Store: "s", ObjectNamespace: e.ObjectNS, ObjectID: e.ObjectID, Relation: e.Relation,
			SubjectNamespace: e.SubjectNS, SubjectID: e.SubjectID, SubjectRelation: e.SubjectRel,
		}
		_ = db.ApplyPut(mvcc.Revision(i+1), tu.Key())
	}
	snap, _ := db.Open(mvcc.Revision(len(edges)))
	defer snap.Close()
	as := authz.Schema{"doc": {
		"can_view":          {Kind: "exclusion", Children: []string{"owner", "banned"}},
		"via_parent":        {Kind: "ttu", Tupleset: "parent", Computed: "viewer"},
		"editor_and_viewer": {Kind: "intersection", Children: []string{"editor", "viewer"}},
	}}
	eng := &authz.Engine{Snap: snap, Schema: as}

	cases := []struct{ rel, sid string }{
		{"can_view", "bob"},   // owner but banned → deny
		{"can_view", "alice"}, // owner not banned → allow
		{"via_parent", "carol"},
		{"via_parent", "eve"},
		{"editor_and_viewer", "dave"},
		{"editor_and_viewer", "alice"},
	}
	for _, c := range cases {
		want := checker.GraphAllowsSchema(schema, edges, "doc", "1", c.rel, "user", c.sid)
		res, err := eng.Check("s", "doc", "1", c.rel, "user", c.sid)
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		got := res == authz.Allow
		if got != want {
			t.Fatalf("%+v: authz=%v oracle=%v", c, got, want)
		}
	}

	ls, err := eng.ListSubjects("s", "doc", "1", "can_view", 32)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range ls {
		if s.SubjectID == "bob" {
			t.Fatal("exclusion ListSubjects must not emit banned bob")
		}
	}
}
