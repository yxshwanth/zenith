package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/raft"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestJointConfigSurvivesCrash(t *testing.T) {
	c := NewCluster(8, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	effs := c.Replicas[leader].Node.ProposeJoint([]runtime.NodeID{1, 2, 4})
	c.handleEffects(leader, effs, c.Sched)
	c.Run(1500)
	// Finalize if joint committed
	if len(c.Replicas[leader].Node.Config().JointWith) > 0 {
		effs = c.Replicas[leader].Node.ProposeFinalize()
		c.handleEffects(leader, effs, c.Sched)
		c.Run(1500)
	}
	cfg := c.Replicas[leader].Node.Config()
	c.Crash(leader)
	leader = c.BootstrapElect(60)
	if leader == 0 {
		t.Fatalf("no leader after crash\n%s", c)
	}
	// Config should remain a valid voter set (G1 gate-minimal).
	got := c.Replicas[leader].Node.Config()
	if len(got.Voters) == 0 {
		t.Fatalf("empty voters after recovery: before=%+v after=%+v", cfg, got)
	}
	_ = raft.Config{}
}
