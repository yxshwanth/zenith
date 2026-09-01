package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/raft"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestJointLiveCrashDualQuorum(t *testing.T) {
	c := NewCluster(81, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.AddNode(4)
	for _, r := range c.Replicas {
		r.Node.SetHoldFinalize(true)
	}
	effs := c.Replicas[leader].Node.ProposeJoint([]runtime.NodeID{1, 2, 4})
	if len(effs) == 0 {
		t.Fatal("ProposeJoint empty")
	}
	c.handleEffects(leader, effs, c.Sched)
	c.Run(2000)
	if len(c.Replicas[leader].Node.Config().JointWith) == 0 {
		t.Fatalf("joint not live at crash: %+v", c.Replicas[leader].Node.Config())
	}
	ids := []runtime.NodeID{1, 2, 3, 4}
	for _, id := range ids {
		c.Crash(id)
		c.Replicas[id].Node.SetHoldFinalize(true)
	}
	leader = c.BootstrapElect(80)
	if leader == 0 {
		t.Fatalf("no leader after joint crash\n%s", c)
	}
	leaders := 0
	for _, r := range c.Replicas {
		if r.Node.Role() == raft.Leader {
			leaders++
		}
	}
	if leaders != 1 {
		t.Fatalf("want 1 leader, got %d\n%s", leaders, c)
	}
	if err := c.prefixOK(); err != nil {
		t.Fatal(err)
	}
	cfg := c.Replicas[leader].Node.Config()
	if len(cfg.JointWith) == 0 {
		t.Logf("joint finalized during recovery cfg=%+v", cfg)
	} else {
		c.Frozen[2] = true
		c.Frozen[3] = true
		c.Frozen[4] = true
		c.ProposePut(leader, "blocked", "s", 1, "joint", "no")
		c.Run(1500)
		if c.Replicas[leader].KV["joint"] == "no" {
			t.Fatalf("committed without dual quorum\n%s", c)
		}
		c.Frozen = map[runtime.NodeID]bool{}
	}
	c.ProposePut(leader, "ok", "s", 2, "alive", "yes")
	c.Run(2000)
	if c.Replicas[leader].KV["alive"] != "yes" && c.CheckHistory().String() == "fail" {
		t.Fatalf("history fail after heal\n%s", c)
	}
}
