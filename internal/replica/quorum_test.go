package replica

import (
	"encoding/json"
	"testing"

	"github.com/yxshwanth/zenith/internal/raft"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestDualQuorumBlocksCommit(t *testing.T) {
	c := NewCluster(71, []runtime.NodeID{1, 2, 3, 4, 5})
	leader := c.BootstrapElect(80)
	if leader == 0 {
		t.Fatal("no leader")
	}
	// Joint: old {1,2,3} new {1,2,4} — freeze 4 so new majority fails if 3 not in new.
	effs := c.Replicas[leader].Node.ProposeJoint([]runtime.NodeID{1, 2, 4})
	c.handleEffects(leader, effs, c.Sched)
	c.Run(500)
	c.Frozen[4] = true
	c.Frozen[3] = true // old set also loses a voter for commits needing both
	before := c.Replicas[leader].Node.CommitIndex()
	c.ProposePut(leader, "x", "s", 1, "k", "v")
	c.Run(1500)
	after := c.Replicas[leader].Node.CommitIndex()
	// With 3 and 4 frozen, neither old nor new has majority beyond leader+one.
	if after > before+5 && c.Replicas[leader].KV["k"] == "v" {
		// may still commit if 1,2 form majority of both {1,2,3} and {1,2,4}
		// old majority: 1,2 of 1,2,3 = yes; new: 1,2 of 1,2,4 = yes. So commit CAN succeed.
		// Partition 2 as well to break both majorities.
	}
	c.Frozen[2] = true
	before = c.Replicas[leader].Node.CommitIndex()
	c.ProposePut(leader, "y", "s", 2, "k2", "v2")
	c.Run(1500)
	if c.Replicas[leader].KV["k2"] == "v2" {
		t.Fatalf("expected dual-quorum block with 2,3,4 frozen\n%s", c)
	}
	_ = before
}

func TestMidJointCrashRecovers(t *testing.T) {
	c := NewCluster(72, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	effs := c.Replicas[leader].Node.ProposeJoint([]runtime.NodeID{1, 2, 3})
	c.handleEffects(leader, effs, c.Sched)
	c.Run(300) // may not finalize yet
	c.Crash(leader)
	leader = c.BootstrapElect(80)
	if leader == 0 {
		t.Fatalf("no leader after mid-joint crash\n%s", c)
	}
	cfg := c.Replicas[leader].Node.Config()
	if len(cfg.Voters) == 0 {
		t.Fatalf("empty voters: %+v", cfg)
	}
}

func TestConfigTruncateRestoresPredecessor(t *testing.T) {
	n := raft.New(1, []runtime.NodeID{1, 2, 3})
	body, _ := json.Marshal(raft.Config{Voters: []runtime.NodeID{1, 2}, JointWith: []runtime.NodeID{1, 2, 3}})
	cmd, _ := json.Marshal(map[string]any{"kind": "Config", "config": json.RawMessage(body)})
	n.ApplyConfig(cmd)
	if len(n.Config().JointWith) == 0 {
		t.Fatal("expected joint")
	}
	body2, _ := json.Marshal(raft.Config{Voters: []runtime.NodeID{1, 2, 3}})
	cmd2, _ := json.Marshal(map[string]any{"kind": "Config", "config": json.RawMessage(body2)})
	n.ApplyConfig(cmd2)
	if len(n.Config().Voters) != 3 || len(n.Config().JointWith) != 0 {
		t.Fatalf("got %+v", n.Config())
	}
}

func TestDropDirAsymmetric(t *testing.T) {
	c := NewCluster(73, []runtime.NodeID{1, 2, 3})
	c.DropDir = map[[2]runtime.NodeID]bool{}
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	// Asymmetric: block leader→2 only via DropDir
	var other runtime.NodeID = 2
	if other == leader {
		other = 3
	}
	c.DropDir[[2]runtime.NodeID{leader, other}] = true
	c.ProposePut(leader, "p", "s", 1, "a", "1")
	c.Run(2000)
	// Still commits via other follower
	if c.Replicas[leader].KV["a"] != "1" {
		t.Fatalf("put should commit\n%s", c)
	}
}
