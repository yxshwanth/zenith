package raft

import (
	"encoding/json"
	"testing"

	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestJointQuorumRequiresBothMajorities(t *testing.T) {
	n := New(1, []runtime.NodeID{1, 2, 3})
	n.cfg = Config{Voters: []runtime.NodeID{1, 2, 4}, JointWith: []runtime.NodeID{1, 2, 3}}
	n.matchIndex = map[runtime.NodeID]uint64{1: 10, 2: 10, 3: 0, 4: 0}
	// old majority yes (1,2), new majority no (only 1,2 of 1,2,4 — that's majority of new actually 2>1.5)
	// new voters 1,2,4: match 1 and 2 => 2*2>3 => true. Both true.
	if !n.quorumCommitted(10) {
		t.Fatal("expected joint quorum")
	}
	n.matchIndex[2] = 0
	// old: only 1 => no; new: only 1 => no
	if n.quorumCommitted(10) {
		t.Fatal("expected no joint quorum")
	}
}

func TestApplyConfigUpdatesPeers(t *testing.T) {
	n := New(1, []runtime.NodeID{1, 2, 3})
	body, _ := jsonMarshalConfig(Config{Voters: []runtime.NodeID{1, 2}})
	cmd, _ := jsonMarshalWrap(body)
	n.ApplyConfig(cmd)
	if len(n.Config().Voters) != 2 || len(n.Config().JointWith) != 0 {
		t.Fatalf("got %+v", n.Config())
	}
}

func jsonMarshalConfig(c Config) ([]byte, error) {
	return json.Marshal(c)
}

func jsonMarshalWrap(body []byte) ([]byte, error) {
	return json.Marshal(map[string]any{"kind": "Config", "config": json.RawMessage(body)})
}
