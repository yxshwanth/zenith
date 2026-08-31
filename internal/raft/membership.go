package raft

import (
	"encoding/json"

	"github.com/yxshwanth/zenith/internal/runtime"
)

// Config is the cluster membership (ADR 0006 / Raft §6).
type Config struct {
	Voters   []runtime.NodeID `json:"voters"`
	Learners []runtime.NodeID `json:"learners,omitempty"`
	// JointWith is non-nil while joint consensus is active (C_old ∪ description).
	JointWith []runtime.NodeID `json:"joint_with,omitempty"`
}

func (n *Node) config() Config {
	if len(n.cfg.Voters) == 0 && len(n.cfg.Learners) == 0 && len(n.cfg.JointWith) == 0 {
		return Config{Voters: append([]runtime.NodeID(nil), n.Peers...)}
	}
	return n.cfg
}

// cfg stored on Node — add field via ensuring we set it in New
func initConfig(peers []runtime.NodeID) Config {
	return Config{Voters: append([]runtime.NodeID(nil), peers...)}
}

func majority(voters []runtime.NodeID, match map[runtime.NodeID]uint64, index uint64) bool {
	if len(voters) == 0 {
		return false
	}
	count := 0
	for _, v := range voters {
		if match[v] >= index {
			count++
		}
	}
	return count*2 > len(voters)
}

func (n *Node) quorumCommitted(index uint64) bool {
	cfg := n.config()
	if len(cfg.JointWith) > 0 {
		return majority(cfg.Voters, n.matchIndex, index) && majority(cfg.JointWith, n.matchIndex, index)
	}
	return majority(cfg.Voters, n.matchIndex, index)
}

func voterSet(cfg Config) map[runtime.NodeID]bool {
	m := map[runtime.NodeID]bool{}
	for _, v := range cfg.Voters {
		m[v] = true
	}
	for _, v := range cfg.JointWith {
		m[v] = true
	}
	return m
}

// ProposeAddLearner appends a non-voting learner (no joint consensus).
func (n *Node) ProposeAddLearner(id runtime.NodeID) []runtime.Effect {
	if n.role != Leader || len(n.cfg.JointWith) > 0 {
		return nil
	}
	cfg := n.config()
	for _, v := range cfg.Voters {
		if v == id {
			return nil
		}
	}
	for _, l := range cfg.Learners {
		if l == id {
			return nil
		}
	}
	learners := append(append([]runtime.NodeID(nil), cfg.Learners...), id)
	body, _ := json.Marshal(Config{Voters: append([]runtime.NodeID(nil), cfg.Voters...), Learners: learners})
	cmd, _ := json.Marshal(map[string]any{"kind": "Config", "config": json.RawMessage(body)})
	idx := n.lastIndex() + 1
	n.log = append(n.log, Entry{Term: n.currentTerm, Index: idx, Command: cmd})
	n.matchIndex[n.ID] = idx
	if n.nextIndex[id] == 0 {
		n.nextIndex[id] = idx
	}
	n.ApplyConfig(cmd)
	return n.persistThen(n.broadcastAppend())
}

// ProposePromote adds a caught-up learner into C_new via joint consensus.
func (n *Node) ProposePromote(id runtime.NodeID) []runtime.Effect {
	if n.role != Leader || len(n.cfg.JointWith) > 0 {
		return nil
	}
	if n.matchIndex[id] < n.commitIndex {
		return nil
	}
	cfg := n.config()
	newVoters := append(append([]runtime.NodeID(nil), cfg.Voters...), id)
	return n.ProposeJoint(newVoters)
}

// ProposeJoint appends a joint config (Cold=current, Cnew=newVoters). Leader only.
func (n *Node) ProposeJoint(newVoters []runtime.NodeID) []runtime.Effect {
	if n.role != Leader || len(n.cfg.JointWith) > 0 {
		return nil
	}
	old := append([]runtime.NodeID(nil), n.config().Voters...)
	learners := filterOut(n.config().Learners, newVoters)
	body, _ := json.Marshal(Config{Voters: newVoters, JointWith: old, Learners: learners})
	cmd, _ := json.Marshal(map[string]any{"kind": "Config", "config": json.RawMessage(body)})
	idx := n.lastIndex() + 1
	n.log = append(n.log, Entry{Term: n.currentTerm, Index: idx, Command: cmd})
	n.matchIndex[n.ID] = idx
	for _, v := range newVoters {
		if n.nextIndex[v] == 0 {
			n.nextIndex[v] = idx
		}
	}
	n.ApplyConfig(cmd)
	n.jointIndex = idx
	return n.persistThen(n.broadcastAppend())
}

// ProposeFinalize appends C_new alone after joint has committed.
func (n *Node) ProposeFinalize() []runtime.Effect {
	if n.role != Leader || len(n.cfg.JointWith) == 0 {
		return nil
	}
	final := Config{
		Voters:   append([]runtime.NodeID(nil), n.cfg.Voters...),
		Learners: append([]runtime.NodeID(nil), n.cfg.Learners...),
	}
	body, _ := json.Marshal(final)
	cmd, _ := json.Marshal(map[string]any{"kind": "Config", "config": json.RawMessage(body)})
	idx := n.lastIndex() + 1
	n.log = append(n.log, Entry{Term: n.currentTerm, Index: idx, Command: cmd})
	n.matchIndex[n.ID] = idx
	n.ApplyConfig(cmd)
	n.jointIndex = 0
	return n.persistThen(n.broadcastAppend())
}

func filterOut(learners, voters []runtime.NodeID) []runtime.NodeID {
	drop := map[runtime.NodeID]bool{}
	for _, v := range voters {
		drop[v] = true
	}
	var out []runtime.NodeID
	for _, l := range learners {
		if !drop[l] {
			out = append(out, l)
		}
	}
	return out
}

// ApplyConfig updates effective configuration from a committed config command.
func (n *Node) ApplyConfig(raw []byte) {
	var wrap struct {
		Kind   string          `json:"kind"`
		Config json.RawMessage `json:"config"`
	}
	if json.Unmarshal(raw, &wrap) != nil || wrap.Kind != "Config" {
		return
	}
	var cfg Config
	if json.Unmarshal(wrap.Config, &cfg) != nil {
		return
	}
	n.cfg = cfg
	n.Peers = nil
	seen := map[runtime.NodeID]bool{}
	for _, v := range cfg.Voters {
		if !seen[v] {
			n.Peers = append(n.Peers, v)
			seen[v] = true
		}
	}
	for _, v := range cfg.JointWith {
		if !seen[v] {
			n.Peers = append(n.Peers, v)
			seen[v] = true
		}
	}
	for _, v := range cfg.Learners {
		if !seen[v] {
			n.Peers = append(n.Peers, v)
			seen[v] = true
		}
	}
	// Removed leader steps down once no longer a voter in the effective config.
	if n.role == Leader && !voterSet(cfg)[n.ID] {
		n.role = Follower
		n.votes = nil
		n.preVotes = nil
	}
}

// IsVoter reports whether id is in the voting set (not learner-only).
func (n *Node) IsVoter(id runtime.NodeID) bool {
	return voterSet(n.config())[id]
}

// Config returns a copy of the effective config.
func (n *Node) Config() Config { return n.config() }

// SetLearnersOnly marks this node as a non-voter pending membership catch-up.
func (n *Node) SetLearnersOnly(id runtime.NodeID) {
	n.cfg = Config{Learners: []runtime.NodeID{id}}
}
