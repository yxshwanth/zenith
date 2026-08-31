package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/raft"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestLearnerNotInQuorum(t *testing.T) {
	c := NewCluster(61, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.AddNode(4)
	effs := c.Replicas[leader].Node.ProposeAddLearner(4)
	c.handleEffects(leader, effs, c.Sched)
	c.Run(2000)

	cfg := c.Replicas[leader].Node.Config()
	found := false
	for _, l := range cfg.Learners {
		if l == 4 {
			found = true
		}
	}
	if !found || c.Replicas[4].Node.IsVoter(4) {
		t.Fatalf("want learner 4 non-voter, cfg=%+v", cfg)
	}

	// Freeze two voters; with learner counted wrongly as peer, old quorum(len(Peers))
	// would break elections — commits must still succeed with majority of voters.
	c.Frozen[4] = true
	c.ProposePut(leader, "p1", "s", 1, "k", "v")
	c.Run(2000)
	if c.Replicas[leader].KV["k"] != "v" {
		t.Fatalf("put should commit without learner ack\n%s", c)
	}
	if c.Replicas[4].Node.Role() == raft.Leader {
		t.Fatal("learner must not become leader")
	}
}

func TestRemoveLeaderUnderLoad(t *testing.T) {
	c := NewCluster(62, []runtime.NodeID{1, 2, 3})
	old := c.BootstrapElect(50)
	if old == 0 {
		t.Fatal("no leader")
	}
	// Load while membership change proceeds.
	c.ProposePut(old, "w0", "s", 1, "a", "1")
	c.Run(800)

	var newVoters []runtime.NodeID
	for _, id := range []runtime.NodeID{1, 2, 3} {
		if id != old {
			newVoters = append(newVoters, id)
		}
	}
	effs := c.Replicas[old].Node.ProposeJoint(newVoters)
	c.handleEffects(old, effs, c.Sched)
	c.ProposePut(old, "w1", "s", 2, "b", "2")
	c.Run(2000)

	// Auto-finalize may have completed; either joint or final without old leader is ok.
	cfgMid := c.Replicas[old].Node.Config()
	if len(cfgMid.JointWith) > 0 {
		if c.Replicas[old].Node.Role() != raft.Leader {
			t.Fatal("leader should remain through joint (still in C_old)")
		}
		effs = c.Replicas[old].Node.ProposeFinalize()
		c.handleEffects(old, effs, c.Sched)
		c.Run(2500)
	}
	c.ProposePut(old, "w2", "s", 3, "c", "3")
	c.Run(1500)

	cfg := c.Replicas[old].Node.Config()
	if len(cfg.JointWith) != 0 || len(cfg.Voters) != 2 {
		t.Fatalf("finalize: %+v\n%s", cfg, c)
	}
	for _, v := range cfg.Voters {
		if v == old {
			t.Fatalf("old leader still voter: %+v", cfg)
		}
	}
	if c.Replicas[old].Node.Role() == raft.Leader {
		t.Fatal("removed leader must step down")
	}

	// Remaining voters elect and serve traffic.
	leader := c.BootstrapElect(80)
	if leader == 0 || leader == old {
		t.Fatalf("want new leader among %v, got %d\n%s", newVoters, leader, c)
	}
	c.ProposePut(leader, "w3", "s", 4, "d", "4")
	c.Run(1500)
	if c.Replicas[leader].KV["d"] != "4" {
		t.Fatalf("post-removal put failed\n%s", c)
	}
	if c.Replicas[leader].KV["a"] != "1" {
		t.Fatalf("lost earlier put under load\n%s", c)
	}
}
