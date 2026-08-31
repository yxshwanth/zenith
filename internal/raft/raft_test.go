package raft

import (
	"encoding/json"
	"testing"

	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestElectionTimeoutStartsPreVote(t *testing.T) {
	n := New(1, []runtime.NodeID{1, 2, 3})
	var effs []runtime.Effect
	for i := 0; i < n.timeoutTicks+2; i++ {
		effs = n.Step(runtime.Tick{Kind: "election"})
		for _, e := range effs {
			if _, ok := e.(runtime.Send); ok {
				return
			}
		}
	}
	t.Fatalf("expected PreVote Send on election timeout, last=%#v", effs)
}

func TestStartElectionPersists(t *testing.T) {
	// Single voter: pre-vote quorum is self → real election + Persist.
	n := New(1, []runtime.NodeID{1})
	var effs []runtime.Effect
	for i := 0; i < n.timeoutTicks+2; i++ {
		effs = n.Step(runtime.Tick{Kind: "election"})
		for _, e := range effs {
			if _, ok := e.(runtime.Persist); ok {
				return
			}
		}
	}
	t.Fatalf("expected Persist on single-voter election, last=%#v", effs)
}

func TestSendAppendInstallsSnapshotWhenCompacted(t *testing.T) {
	n := New(1, []runtime.NodeID{1, 2, 3})
	n.role = Leader
	n.currentTerm = 1
	data := []byte(`{"meta":{"lastIndex":5,"lastTerm":1},"kv":{"k":"v"}}`)
	n.CompactLog(5, 1)
	n.SetSnapData(data)
	n.nextIndex[2] = 1
	effs := n.sendAppend(2)
	if len(effs) != 1 {
		t.Fatalf("want 1 send, got %d", len(effs))
	}
	s, ok := effs[0].(runtime.Send)
	if !ok {
		t.Fatalf("want Send, got %T", effs[0])
	}
	var m peerMsg
	if err := json.Unmarshal(s.Payload, &m); err != nil {
		t.Fatal(err)
	}
	if m.Type != msgInstallSnapshot {
		t.Fatalf("want InstallSnapshot, got %s", m.Type)
	}
	if string(m.SnapData) != string(data) {
		t.Fatalf("snap data mismatch")
	}
}
