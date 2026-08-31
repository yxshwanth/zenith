package replica

import (
	"testing"

	v2 "github.com/yxshwanth/zenith/api/zenith/v2"
	"github.com/yxshwanth/zenith/internal/deccache"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestCheckUsesDecCacheAtSameRevision(t *testing.T) {
	c := NewCluster(80, []runtime.NodeID{1, 2, 3})
	c.DecCache = deccache.New()
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.ProposeTuple(leader, "t1", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "a",
	})
	c.Run(1000)
	req := v2.CheckRequest{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "a",
		Consistency: v2.Consistency{Mode: v2.FullyConsistent},
	}
	r1 := c.CheckPermission(leader, req)
	if !r1.Allowed {
		t.Fatal(r1)
	}
	// Second check at same applied should hit cache (still ALLOW).
	r2 := c.CheckPermission(leader, req)
	if !r2.Allowed {
		t.Fatal(r2)
	}
}
