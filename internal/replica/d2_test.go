package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestSessionUnknownRetryThenDigestMismatch(t *testing.T) {
	c := NewCluster(15, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	for _, p := range []runtime.NodeID{1, 2, 3} {
		if p != leader {
			c.Partition(leader, p)
		}
	}
	c.ProposePut(leader, "u1", "sess", 1, "k", "v")
	c.Run(400)
	acked := false
	for _, h := range c.History {
		if h.Op.Key == "k" && !h.Unknown {
			acked = true
		}
	}
	if acked {
		t.Fatal("put should be unknown while leader is partitioned")
	}
	for _, p := range []runtime.NodeID{1, 2, 3} {
		if p != leader {
			c.Heal(leader, p)
		}
	}
	c.ProposePut(leader, "u1-retry", "sess", 1, "k", "v")
	c.Run(1500)
	if c.Replicas[leader].KV["k"] != "v" {
		t.Fatalf("retry same identity should apply once\n%s", c)
	}
	c.ProposePut(leader, "u1-evil", "sess", 1, "k", "EVIL")
	c.Run(1500)
	if c.Replicas[leader].KV["k"] != "v" {
		t.Fatalf("different digest must not overwrite: %s", c)
	}
	for _, h := range c.History {
		if h.Op.Value == "EVIL" && !h.Unknown {
			t.Fatal("digest mismatch must not be recorded as a successful put")
		}
	}
	if out := c.CheckHistory(); out == checker.Fail {
		t.Fatalf("history %s: %+v", out, c.History)
	}
}
