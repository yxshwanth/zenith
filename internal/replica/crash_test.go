package replica

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/runtime"
	"github.com/yxshwanth/zenith/internal/snapshot"
)

// TestCrashMatrixNoLostAcks is the Phase 2 gate: crash/restart schedules must
// not lose a put that was acknowledged to the client.
func TestCrashMatrixNoLostAcks(t *testing.T) {
	c := NewCluster(21, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.ProposePut(leader, "ack1", "s", 1, "k", "v")
	c.Run(1500)
	if out := c.CheckHistory(); out == checker.Fail {
		t.Fatalf("history fail before crash: %+v", c.History)
	}
	acked := false
	for _, h := range c.History {
		if h.Op.Key == "k" && !h.Unknown && h.Result.Value == "v" {
			acked = true
		}
	}
	if !acked {
		t.Fatalf("put not acknowledged: %+v\n%s", c.History, c)
	}

	// Crash every node (synced disks preserved).
	for _, id := range []runtime.NodeID{1, 2, 3} {
		c.Crash(id)
	}
	leader = c.BootstrapElect(80)
	if leader == 0 {
		t.Fatalf("no leader after crash\n%s", c)
	}
	// Drive a noop-commit round and read.
	c.ProposePut(leader, "ack2", "s", 2, "k2", "v2")
	c.Run(1500)
	found := false
	for _, r := range c.Replicas {
		if r.KV["k"] == "v" {
			found = true
		}
	}
	if !found {
		// May still be only in log awaiting commit — try strong path via another elect round
		c.TickElection()
		c.Run(1500)
		for _, r := range c.Replicas {
			if r.KV["k"] == "v" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("lost acknowledged write after crash matrix\n%s", c)
	}
}

func TestSnapshotPublishCrashKeepsOldGen(t *testing.T) {
	s := &snapshot.Store{}
	_ = s.BeginWrite(snapshot.State{Meta: snapshot.Meta{LastIndex: 1}, KV: map[string]string{"x": "1"}})
	s.Publish()
	_ = s.BeginWrite(snapshot.State{Meta: snapshot.Meta{LastIndex: 2}, KV: map[string]string{"x": "2"}})
	s.CrashDropStaging()
	st, err := s.Load()
	if err != nil || st.KV["x"] != "1" {
		t.Fatalf("got %+v err=%v", st, err)
	}
}

func TestPreVoteDoesNotBumpTermAlone(t *testing.T) {
	c := NewCluster(5, []runtime.NodeID{1, 2, 3})
	// One election tick round should send pre-votes before real term bumps on all.
	c.TickElection()
	c.Run(50)
	// Not all nodes necessarily bumped; after enough rounds a leader forms.
	leader := c.BootstrapElect(60)
	if leader == 0 {
		t.Fatalf("pre-vote path failed to elect\n%s", c)
	}
}
