package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/authz"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestGCAfterApplyKeepsCheck(t *testing.T) {
	c := NewCluster(5, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.ProposeTuple(leader, "t1", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob",
	})
	c.Run(800)
	db := c.Replicas[leader].MVCC
	if err := db.GC(db.Applied()); err != nil {
		t.Fatal(err)
	}
	snap, err := db.Open(db.Applied())
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	e := &authz.Engine{Snap: snap}
	res, err := e.Check("s", "doc", "1", "viewer", "user", "bob")
	if err != nil || res != authz.Allow {
		t.Fatalf("after GC got %v %v", res, err)
	}
}
