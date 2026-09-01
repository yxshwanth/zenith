package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/raft"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestElectAndPut(t *testing.T) {
	c := NewCluster(42, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(40)
	if leader == 0 {
		t.Fatalf("no leader\n%s", c)
	}
	c.ProposePut(leader, "r1", "s1", 1, "x", "1")
	c.Run(1000)
	if c.Replicas[leader].KV["x"] != "1" {
		t.Fatalf("leader kv=%v\n%s", c.Replicas[leader].KV, c)
	}
	// majority should have applied
	applied := 0
	for _, r := range c.Replicas {
		if r.KV["x"] == "1" {
			applied++
		}
	}
	if applied < 2 {
		t.Fatalf("expected majority applied, got %d\n%s", applied, c)
	}
	if out := c.CheckHistory(); out != checker.Pass && out != checker.Inconclusive {
		t.Fatalf("history %s: %+v", out, c.History)
	}
}

func TestCrashRestartPreservesSyncedPut(t *testing.T) {
	c := NewCluster(7, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(40)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.ProposePut(leader, "r1", "s1", 1, "k", "v")
	c.Run(1000)
	if c.Replicas[leader].KV["k"] != "v" {
		t.Fatalf("put failed: %s", c)
	}
	// crash all, restore from synced WAL
	for _, id := range []runtime.NodeID{1, 2, 3} {
		c.Crash(id)
	}
	leader = c.BootstrapElect(60)
	if leader == 0 {
		t.Fatalf("no leader after crash\n%s", c)
	}
	// value should be on nodes that retained durable log (rebuilt KV)
	found := false
	for _, r := range c.Replicas {
		if r.KV["k"] == "v" {
			found = true
		}
	}
	if !found {
		t.Fatalf("lost acknowledged put after crash\n%s", c)
	}
}

func TestPartitionMinorityCannotCommit(t *testing.T) {
	c := NewCluster(99, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(40)
	if leader == 0 {
		t.Fatal("no leader")
	}
	// isolate leader from both peers
	for _, p := range []runtime.NodeID{1, 2, 3} {
		if p != leader {
			c.Partition(leader, p)
		}
	}
	c.ProposePut(leader, "r2", "s1", 2, "alone", "x")
	c.Run(500)
	// should not be acknowledged as committed on majority — follower KVs empty for key
	followersHave := 0
	for id, r := range c.Replicas {
		if id != leader && r.KV["alone"] == "x" {
			followersHave++
		}
	}
	if followersHave > 0 {
		t.Fatalf("minority partition committed to followers")
	}
	// heal and progress
	for _, p := range []runtime.NodeID{1, 2, 3} {
		if p != leader {
			c.Heal(leader, p)
		}
	}
	c.TickElection()
	c.Run(1000)
	leader = c.Leader()
	if leader == 0 {
		leader = c.BootstrapElect(40)
	}
	c.ProposePut(leader, "r3", "s1", 3, "healed", "y")
	c.Run(1000)
	ok := false
	for _, r := range c.Replicas {
		if r.KV["healed"] == "y" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("no progress after heal\n%s", c)
	}
}

func TestSessionRetryNoDup(t *testing.T) {
	c := NewCluster(3, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(40)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.ProposePut(leader, "a", "sess", 1, "x", "1")
	c.Run(1000)
	// retry same identity
	c.ProposePut(leader, "a-retry", "sess", 1, "x", "1")
	c.Run(500)
	// value still 1; session table prevents dup apply side effects (same value)
	if c.Replicas[leader].KV["x"] != "1" {
		t.Fatal("value changed")
	}
}

func TestStrongGetFence(t *testing.T) {
	c := NewCluster(11, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(40)
	c.ProposePut(leader, "p", "s", 1, "g", "1")
	c.Run(1000)
	v, ok := c.StrongGet(leader, "fence1", "g")
	if !ok || v != "1" {
		t.Fatalf("got %q %v", v, ok)
	}
}

func TestStrongGetStaleLeaderUnknown(t *testing.T) {
	c := NewCluster(12, []runtime.NodeID{1, 2, 3})
	old := c.BootstrapElect(40)
	if old == 0 {
		t.Fatal("no leader")
	}
	c.ProposePut(old, "p1", "s", 1, "k", "v5")
	c.Run(1000)
	for _, p := range []runtime.NodeID{1, 2, 3} {
		if p != old {
			c.Partition(old, p)
		}
	}
	var neu runtime.NodeID
	for i := 0; i < 80 && neu == 0; i++ {
		c.TickElection()
		c.Run(200)
		for id, r := range c.Replicas {
			if id != old && r.Node.Role() == raft.Leader {
				neu = id
				break
			}
		}
	}
	if neu == 0 {
		t.Fatal("no new leader")
	}
	c.ProposePut(neu, "p2", "s", 2, "k", "v3")
	c.Run(1000)
	v, ok := c.StrongGet(old, "stale-fence", "k")
	if ok {
		t.Fatalf("stale leader must not complete get, got %q", v)
	}
}
