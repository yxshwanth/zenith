package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/raft"
	"github.com/yxshwanth/zenith/internal/runtime"
)

// TestR1AtMostOneLeaderPerTerm elects a cluster and checks a single Leader role.
func TestR1AtMostOneLeaderPerTerm(t *testing.T) {
	c := NewCluster(17, []runtime.NodeID{1, 2, 3})
	_ = c.BootstrapElect(50)
	leaders := 0
	var term uint64
	for _, r := range c.Replicas {
		if r.Node.Role() == raft.Leader {
			leaders++
			term = r.Node.Term()
		}
	}
	if leaders != 1 {
		t.Fatalf("want 1 leader, got %d\n%s", leaders, c)
	}
	for _, r := range c.Replicas {
		if r.Node.Role() == raft.Leader && r.Node.Term() != term {
			t.Fatal("leader term mismatch")
		}
	}
}
