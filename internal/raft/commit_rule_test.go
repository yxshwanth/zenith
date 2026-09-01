package raft

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func prevTermMajorityLeader() *Node {
	n := New(1, []runtime.NodeID{1, 2, 3})
	n.role = Leader
	n.currentTerm = 2
	n.log = []Entry{
		{Term: 0, Index: 0},
		{Term: 1, Index: 1, Command: []byte(`{"kind":"Put","key":"x","value":"old"}`)},
	}
	n.matchIndex[1] = 1
	n.matchIndex[2] = 1
	n.matchIndex[3] = 0
	n.nextIndex[2] = 2
	n.nextIndex[3] = 2
	return n
}

func TestMaybeCommitRefusesPreviousTermWithoutCurrentTermEntry(t *testing.T) {
	n := prevTermMajorityLeader()
	n.maybeCommit()
	if n.commitIndex != 0 {
		t.Fatalf("production commit rule committed index %d from term 1 in term 2", n.commitIndex)
	}
}

func TestBrokenCommitRuleCommitsPreviousTermAndCheckerFails(t *testing.T) {
	n := prevTermMajorityLeader()
	n.SetCommitAnyTerm(true)
	effs := n.maybeCommit()
	if n.commitIndex != 1 {
		t.Fatalf("broken rule should commit index 1, got %d", n.commitIndex)
	}
	sawApply := false
	for _, e := range effs {
		if a, ok := e.(runtime.Apply); ok && a.Index == 1 {
			sawApply = true
		}
	}
	if !sawApply {
		t.Fatalf("broken rule should Apply index 1, got %#v", effs)
	}
	// Client observed the prev-term put as committed, then a Get of a
	// value that only exists if that slot was overwritten (Figure 8 class).
	hist := []checker.HistoryEntry{
		{
			Op:        checker.KVOp{Kind: checker.KVPut, Key: "x", Value: "old"},
			Result:    checker.KVResult{Found: true, Value: "old"},
			InvokedAt: 0, CompletedAt: 10,
		},
		{
			Op:        checker.KVOp{Kind: checker.KVGet, Key: "x"},
			Result:    checker.KVResult{Found: true, Value: "new"},
			InvokedAt: 20, CompletedAt: 30,
		},
	}
	if checker.CheckKVPorcupine(hist) != checker.Fail {
		t.Fatal("Porcupine must reject committed-then-overwritten history")
	}
	n2 := prevTermMajorityLeader()
	n2.maybeCommit()
	if n2.commitIndex != 0 {
		t.Fatal("production rule must still refuse")
	}
}
