package checker

import "testing"

func TestVisibleAtOracle(t *testing.T) {
	ops := []VersionOp{
		{Rev: 1, Key: "k", Present: true},
		{Rev: 3, Key: "k", Present: false},
	}
	if !VisibleAt(ops, "k", 1) || !VisibleAt(ops, "k", 2) {
		t.Fatal("should be present before tombstone")
	}
	if VisibleAt(ops, "k", 3) {
		t.Fatal("tombstone at 3")
	}
}

func TestGraphOracleUserset(t *testing.T) {
	edges := []GraphEdge{
		{ObjectNS: "doc", ObjectID: "1", Relation: "viewer", SubjectNS: "group", SubjectID: "eng", SubjectRel: "member"},
		{ObjectNS: "group", ObjectID: "eng", Relation: "member", SubjectNS: "user", SubjectID: "bob"},
	}
	if !GraphAllows(edges, "doc", "1", "viewer", "user", "bob") {
		t.Fatal("expected allow")
	}
	if GraphAllows(edges, "doc", "1", "viewer", "user", "eve") {
		t.Fatal("eve should deny")
	}
}
