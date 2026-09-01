package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestCrashDuringInstallSnapshotKeepsPrefix(t *testing.T) {
	c := NewCluster(13, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.ProposePut(leader, "p1", "s", 1, "k", "v")
	c.Run(1500)
	var lag runtime.NodeID
	for _, id := range []runtime.NodeID{1, 2, 3} {
		if id != leader {
			lag = id
			break
		}
	}
	c.Frozen[lag] = true
	c.ProposePut(leader, "p2", "s", 2, "k2", "v2")
	c.Run(1500)
	c.publishSnapshot(leader)
	if c.Replicas[leader].Node.SnapIndex() == 0 {
		t.Fatal("leader did not compact")
	}
	delete(c.Frozen, lag)
	c.Run(5)
	c.Crash(lag)
	leader2 := c.BootstrapElect(80)
	if leader2 == 0 {
		t.Fatalf("no leader after install crash\n%s", c)
	}
	if err := c.prefixOK(); err != nil {
		t.Fatal(err)
	}
	if c.Replicas[lag].Node.LastApplied() > c.Replicas[leader2].Node.LastApplied()+8 {
		t.Fatalf("lag applied %d ahead of leader %d", c.Replicas[lag].Node.LastApplied(), c.Replicas[leader2].Node.LastApplied())
	}
	found := false
	for _, r := range c.Replicas {
		if r.KV["k"] == "v" {
			found = true
		}
	}
	if !found {
		c.ProposePut(leader2, "p3", "s", 3, "k3", "v3")
		c.Run(1500)
		for _, r := range c.Replicas {
			if r.KV["k"] == "v" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("lost k=v after mid-install crash\n%s", c)
	}
	if out := c.CheckHistory(); out == checker.Fail {
		t.Fatalf("history fail: %+v", c.History)
	}
}
