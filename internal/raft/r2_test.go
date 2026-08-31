package raft

import (
	"encoding/json"
	"testing"

	"github.com/yxshwanth/zenith/internal/runtime"
)

// TestR2VotePersistsBeforeGrantVisible: Persist/Sync before a granted vote is sent.
func TestR2VotePersistsBeforeGrantVisible(t *testing.T) {
	n := New(1, []runtime.NodeID{1, 2, 3})
	msg, _ := json.Marshal(peerMsg{
		Type: msgRequestVote, Term: 1, From: 2, Candidate: 2,
		LastLogIndex: 0, LastLogTerm: 0,
	})
	effs := n.Step(runtime.PeerMessage{From: 2, Payload: msg})
	var sawPersist, sawSend bool
	for _, e := range effs {
		switch e.(type) {
		case runtime.Persist:
			sawPersist = true
		case runtime.Send:
			if sawPersist {
				sawSend = true
			}
		}
	}
	if !sawPersist {
		t.Fatal("expected Persist before vote grant (R2)")
	}
	// Send is deferred until after Sync in persistThen — grant goes in afterSync.
	if sawSend {
		t.Fatal("Send must not precede Sync completion")
	}
	var syncID string
	for _, e := range effs {
		if s, ok := e.(runtime.Sync); ok {
			syncID = s.RequestID
		}
	}
	after := n.Step(runtime.DiskCompletion{RequestID: syncID, Synced: true})
	for _, e := range after {
		if _, ok := e.(runtime.Send); ok {
			return
		}
	}
	t.Fatalf("expected Send after Sync, got %#v", after)
}
