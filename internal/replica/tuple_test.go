package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/authz"
	"github.com/yxshwanth/zenith/internal/mvcc"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestTupleApplyThenCheck(t *testing.T) {
	c := NewCluster(33, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.ProposeTuple(leader, "t1", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob",
	})
	c.Run(1500)
	db := c.Replicas[leader].MVCC
	snap, err := db.Open(db.Applied())
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	e := &authz.Engine{Snap: snap}
	res, err := e.Check("s", "doc", "1", "viewer", "user", "bob")
	if err != nil || res != authz.Allow {
		t.Fatalf("got %v %v (applied=%d)", res, err, db.Applied())
	}
	k := mvcc.EncodeKey("s", "doc", "1", "viewer", "user", "bob", "")
	ok, _ := snap.Contains(k)
	if !ok {
		t.Fatal("tuple missing in MVCC")
	}
}
