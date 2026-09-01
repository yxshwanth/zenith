// Package raft is a minimal Raft Core (docs/adr/0001-custom-raft-implementation.md).
// Three voters; pre-vote + snapshot install (Phase 2). Membership is Phase 5.
//
// ponytail: one in-flight Persist/Sync per node; peer codec is JSON.
package raft

import (
	"encoding/json"
	"fmt"

	"github.com/yxshwanth/zenith/internal/runtime"
)

type Role int

const (
	Follower Role = iota
	Candidate
	Leader
)

// Entry is one Raft log record.
type Entry struct {
	Term    uint64 `json:"term"`
	Index   uint64 `json:"index"`
	Command []byte `json:"command"`
}

type msgType string

const (
	msgRequestVote         msgType = "RequestVote"
	msgRequestVoteResp     msgType = "RequestVoteResp"
	msgAppendEntries       msgType = "AppendEntries"
	msgAppendEntriesResp   msgType = "AppendEntriesResp"
	msgPreVote             msgType = "PreVote"
	msgPreVoteResp         msgType = "PreVoteResp"
	msgInstallSnapshot     msgType = "InstallSnapshot"
	msgInstallSnapshotResp msgType = "InstallSnapshotResp"
)

type peerMsg struct {
	Type         msgType        `json:"type"`
	Term         uint64         `json:"term"`
	From         runtime.NodeID `json:"from"`
	To           runtime.NodeID `json:"to"`
	Candidate    runtime.NodeID `json:"candidate,omitempty"`
	LastLogIndex uint64         `json:"lastLogIndex,omitempty"`
	LastLogTerm  uint64         `json:"lastLogTerm,omitempty"`
	VoteGranted  bool           `json:"voteGranted,omitempty"`
	PrevLogIndex uint64         `json:"prevLogIndex,omitempty"`
	PrevLogTerm  uint64         `json:"prevLogTerm,omitempty"`
	Entries      []Entry        `json:"entries,omitempty"`
	LeaderCommit uint64         `json:"leaderCommit,omitempty"`
	Success      bool           `json:"success,omitempty"`
	MatchIndex   uint64         `json:"matchIndex,omitempty"`
	Durable      bool           `json:"durable,omitempty"`
	SnapIndex    uint64         `json:"snapIndex,omitempty"`
	SnapTerm     uint64         `json:"snapTerm,omitempty"`
	SnapData     []byte         `json:"snapData,omitempty"`
}

// hardState is what we Persist.
type hardState struct {
	Term     uint64         `json:"term"`
	VotedFor runtime.NodeID `json:"votedFor"`
	Log      []Entry        `json:"log"`
	SnapIdx  uint64         `json:"snapIdx,omitempty"`
	SnapTerm uint64         `json:"snapTerm,omitempty"`
	Config   Config         `json:"config,omitempty"`
}

// Node is a deterministic Raft replica Core.
type Node struct {
	ID    runtime.NodeID
	Peers []runtime.NodeID

	currentTerm uint64
	votedFor    runtime.NodeID
	log         []Entry

	commitIndex uint64
	lastApplied uint64
	role        Role

	nextIndex  map[runtime.NodeID]uint64
	matchIndex map[runtime.NodeID]uint64
	votes      map[runtime.NodeID]bool
	preVotes   map[runtime.NodeID]bool

	syncInFlight bool
	persistID    string
	afterSync    []runtime.Effect

	waiting map[string]uint64
	inbox   []runtime.ClientCommand

	electionTicks int
	timeoutTicks  int
	tickCount     int

	snapIndex     uint64
	snapTerm      uint64
	snapData      []byte
	pendingSnap   []byte
	cfg           Config
	bootstrap     []runtime.NodeID // initial voters for truncate rebuild
	jointIndex    uint64           // log index of active joint entry; 0 if none
	commitAnyTerm bool             // test-only: skip Raft §5.4.2 current-term check
	holdFinalize  bool             // test-only: keep C_old,new live (no auto ProposeFinalize)
}

// New returns a follower with empty log and a dummy entry at index 0.
func New(id runtime.NodeID, peers []runtime.NodeID) *Node {
	n := &Node{
		ID:           id,
		Peers:        peers,
		log:          []Entry{{Term: 0, Index: 0}},
		votedFor:     0,
		role:         Follower,
		nextIndex:    map[runtime.NodeID]uint64{},
		matchIndex:   map[runtime.NodeID]uint64{},
		votes:        map[runtime.NodeID]bool{},
		preVotes:     map[runtime.NodeID]bool{},
		waiting:      map[string]uint64{},
		timeoutTicks: 3 + int(id)*3,
		cfg:          initConfig(peers),
		bootstrap:    append([]runtime.NodeID(nil), peers...),
	}
	return n
}

// SetElectionTimeout sets the follower campaign threshold in ticks.
func (n *Node) SetElectionTimeout(ticks int) {
	if ticks < 1 {
		ticks = 1
	}
	n.timeoutTicks = ticks
}

// SetCommitAnyTerm enables the broken commit rule (previous-term entries
// become committed without a current-term entry). Production stays false.
func (n *Node) SetCommitAnyTerm(broken bool) { n.commitAnyTerm = broken }

// SetHoldFinalize keeps joint consensus from auto-finalizing (G1 crash tests).
func (n *Node) SetHoldFinalize(v bool) { n.holdFinalize = v }

func (n *Node) lastIndex() uint64 { return n.log[len(n.log)-1].Index }
func (n *Node) lastTerm() uint64  { return n.log[len(n.log)-1].Term }

func (n *Node) entry(i uint64) (Entry, bool) {
	if i == 0 {
		return n.log[0], true
	}
	if i >= uint64(len(n.log)) {
		return Entry{}, false
	}
	e := n.log[i]
	if e.Index != i {
		// compact denseness invariant for Phase 1
		for _, x := range n.log {
			if x.Index == i {
				return x, true
			}
		}
		return Entry{}, false
	}
	return e, true
}

// Step implements runtime.Core.
func (n *Node) Step(ev runtime.Event) []runtime.Effect {
	switch e := ev.(type) {
	case runtime.Tick:
		return n.onTick(e)
	case runtime.PeerMessage:
		return n.onPeer(e)
	case runtime.DiskCompletion:
		return n.onDisk(e)
	case runtime.ClientCommand:
		return n.onClient(e)
	default:
		return nil
	}
}

func (n *Node) onTick(t runtime.Tick) []runtime.Effect {
	if t.Kind != "election" && t.Kind != "heartbeat" && t.Kind != "" {
		return nil
	}
	n.tickCount++
	if n.role == Leader {
		return n.broadcastAppend()
	}
	if !n.IsVoter(n.ID) {
		return nil // learners do not campaign
	}
	n.electionTicks++
	if n.electionTicks >= n.timeoutTicks {
		return n.startPreVote()
	}
	return nil
}

// startPreVote probes whether an election would succeed without incrementing term.
func (n *Node) startPreVote() []runtime.Effect {
	n.preVotes = map[runtime.NodeID]bool{n.ID: true}
	n.electionTicks = 0
	var out []runtime.Effect
	prospective := n.currentTerm + 1
	for _, p := range n.votingPeers() {
		if p == n.ID {
			continue
		}
		out = append(out, n.send(p, peerMsg{
			Type:         msgPreVote,
			Term:         prospective, // prospective term; must not bump currentTerm
			From:         n.ID,
			Candidate:    n.ID,
			LastLogIndex: n.lastIndex(),
			LastLogTerm:  n.lastTerm(),
		})...)
	}
	if n.electionQuorum(n.preVotes) {
		return n.startElection()
	}
	return out
}

func (n *Node) startElection() []runtime.Effect {
	n.role = Candidate
	n.currentTerm++
	n.votedFor = n.ID
	n.votes = map[runtime.NodeID]bool{n.ID: true}
	n.electionTicks = 0
	return n.persistThen(n.voteRequests())
}

func (n *Node) becomeLeader() []runtime.Effect {
	n.role = Leader
	li := n.lastIndex()
	for _, p := range n.Peers {
		n.nextIndex[p] = li + 1
		n.matchIndex[p] = 0
	}
	// No-op commits prior-term entries (Raft §5.4.2).
	idx := li + 1
	n.log = append(n.log, Entry{
		Term:    n.currentTerm,
		Index:   idx,
		Command: []byte(`{"kind":"Noop"}`),
	})
	n.matchIndex[n.ID] = idx
	n.nextIndex[n.ID] = idx + 1
	return n.persistThen(n.broadcastAppend())
}

func (n *Node) voteRequests() []runtime.Effect {
	var out []runtime.Effect
	for _, p := range n.votingPeers() {
		if p == n.ID {
			continue
		}
		out = append(out, n.send(p, peerMsg{
			Type:         msgRequestVote,
			Term:         n.currentTerm,
			From:         n.ID,
			To:           p,
			Candidate:    n.ID,
			LastLogIndex: n.lastIndex(),
			LastLogTerm:  n.lastTerm(),
		})...)
	}
	if n.electionQuorum(n.votes) {
		out = append(out, n.becomeLeader()...)
	}
	return out
}

func (n *Node) onClient(cmd runtime.ClientCommand) []runtime.Effect {
	if n.role != Leader {
		return []runtime.Effect{runtime.Reply{
			RequestID: cmd.RequestID,
			Err:       fmt.Errorf("not leader"),
		}}
	}
	if n.syncInFlight {
		n.inbox = append(n.inbox, cmd)
		return nil
	}
	idx := n.lastIndex() + 1
	n.log = append(n.log, Entry{Term: n.currentTerm, Index: idx, Command: append([]byte(nil), cmd.Payload...)})
	n.waiting[cmd.RequestID] = idx
	n.matchIndex[n.ID] = idx
	return n.persistThen(n.broadcastAppend())
}

func (n *Node) broadcastAppend() []runtime.Effect {
	var out []runtime.Effect
	for _, p := range n.Peers {
		if p == n.ID {
			continue
		}
		out = append(out, n.sendAppend(p)...)
	}
	old := n.commitIndex
	out = append(out, n.maybeCommit()...)
	if n.commitIndex > old {
		// Propagate LeaderCommit without re-entering maybeCommit recursion.
		for _, p := range n.Peers {
			if p == n.ID {
				continue
			}
			out = append(out, n.sendAppend(p)...)
		}
	}
	return out
}

func (n *Node) sendAppend(to runtime.NodeID) []runtime.Effect {
	next := n.nextIndex[to]
	if next == 0 {
		next = 1
	}
	if n.snapIndex > 0 && next <= n.snapIndex && len(n.snapData) > 0 {
		return n.SendInstallSnapshot(to, n.snapData)
	}
	prevIdx := next - 1
	prevTerm := uint64(0)
	if e, ok := n.entry(prevIdx); ok {
		prevTerm = e.Term
	}
	var entries []Entry
	for _, e := range n.log {
		if e.Index >= next {
			entries = append(entries, e)
		}
	}
	return n.send(to, peerMsg{
		Type:         msgAppendEntries,
		Term:         n.currentTerm,
		From:         n.ID,
		To:           to,
		PrevLogIndex: prevIdx,
		PrevLogTerm:  prevTerm,
		Entries:      entries,
		LeaderCommit: n.commitIndex,
	})
}

func (n *Node) onPeer(pm runtime.PeerMessage) []runtime.Effect {
	var m peerMsg
	if err := json.Unmarshal(pm.Payload, &m); err != nil {
		return nil
	}
	// Pre-vote uses a prospective term and must not bump currentTerm.
	if m.Type != msgPreVote && m.Type != msgPreVoteResp && m.Term > n.currentTerm {
		n.becomeFollower(m.Term)
	}
	switch m.Type {
	case msgPreVote:
		return n.onPreVote(m)
	case msgPreVoteResp:
		return n.onPreVoteResp(m)
	case msgRequestVote:
		return n.onRequestVote(m)
	case msgRequestVoteResp:
		return n.onRequestVoteResp(m)
	case msgAppendEntries:
		return n.onAppendEntries(m)
	case msgAppendEntriesResp:
		return n.onAppendEntriesResp(m)
	case msgInstallSnapshot:
		return n.onInstallSnapshot(m)
	case msgInstallSnapshotResp:
		return n.onInstallSnapshotResp(m)
	}
	return nil
}

func (n *Node) becomeFollower(term uint64) {
	n.currentTerm = term
	n.role = Follower
	n.votedFor = 0
	n.votes = nil
	n.preVotes = nil
	n.electionTicks = 0
}

func (n *Node) onPreVote(m peerMsg) []runtime.Effect {
	// Pre-vote must not advance currentTerm.
	if m.Term < n.currentTerm+1 {
		return n.send(m.From, peerMsg{Type: msgPreVoteResp, Term: n.currentTerm, From: n.ID, VoteGranted: false})
	}
	upToDate := m.LastLogTerm > n.lastTerm() || (m.LastLogTerm == n.lastTerm() && m.LastLogIndex >= n.lastIndex())
	grant := upToDate && n.role != Leader
	return n.send(m.From, peerMsg{Type: msgPreVoteResp, Term: m.Term, From: n.ID, VoteGranted: grant})
}

func (n *Node) onPreVoteResp(m peerMsg) []runtime.Effect {
	if m.Term != n.currentTerm+1 || !m.VoteGranted {
		return nil
	}
	if n.preVotes == nil {
		return nil
	}
	n.preVotes[m.From] = true
	if n.electionQuorum(n.preVotes) {
		return n.startElection()
	}
	return nil
}

func (n *Node) onRequestVote(m peerMsg) []runtime.Effect {
	if m.Term < n.currentTerm {
		return n.send(m.From, peerMsg{Type: msgRequestVoteResp, Term: n.currentTerm, From: n.ID, VoteGranted: false})
	}
	if !n.IsVoter(n.ID) {
		return n.send(m.From, peerMsg{Type: msgRequestVoteResp, Term: n.currentTerm, From: n.ID, VoteGranted: false})
	}
	upToDate := m.LastLogTerm > n.lastTerm() || (m.LastLogTerm == n.lastTerm() && m.LastLogIndex >= n.lastIndex())
	grant := (n.votedFor == 0 || n.votedFor == m.Candidate) && upToDate
	if grant {
		n.votedFor = m.Candidate
		n.electionTicks = 0
		return n.persistThen(n.send(m.From, peerMsg{Type: msgRequestVoteResp, Term: n.currentTerm, From: n.ID, VoteGranted: true}))
	}
	return n.send(m.From, peerMsg{Type: msgRequestVoteResp, Term: n.currentTerm, From: n.ID, VoteGranted: false})
}

func (n *Node) onRequestVoteResp(m peerMsg) []runtime.Effect {
	if n.role != Candidate || m.Term != n.currentTerm {
		return nil
	}
	if m.VoteGranted {
		n.votes[m.From] = true
		if n.electionQuorum(n.votes) {
			return n.becomeLeader()
		}
	}
	return nil
}

func (n *Node) onAppendEntries(m peerMsg) []runtime.Effect {
	if m.Term < n.currentTerm {
		return n.send(m.From, peerMsg{Type: msgAppendEntriesResp, Term: n.currentTerm, From: n.ID, Success: false})
	}
	n.role = Follower
	n.electionTicks = 0
	// log consistency
	if m.PrevLogIndex > n.lastIndex() {
		return n.persistThen(n.send(m.From, peerMsg{Type: msgAppendEntriesResp, Term: n.currentTerm, From: n.ID, Success: false, MatchIndex: n.lastIndex()}))
	}
	if m.PrevLogIndex > 0 {
		e, ok := n.entry(m.PrevLogIndex)
		if !ok || e.Term != m.PrevLogTerm {
			// truncate conflict
			n.truncateFrom(m.PrevLogIndex)
			return n.persistThen(n.send(m.From, peerMsg{Type: msgAppendEntriesResp, Term: n.currentTerm, From: n.ID, Success: false, MatchIndex: n.lastIndex()}))
		}
	}
	for _, ent := range m.Entries {
		if existing, ok := n.entry(ent.Index); ok {
			if existing.Term != ent.Term {
				n.truncateFrom(ent.Index)
				n.log = append(n.log, ent)
				n.ApplyConfig(ent.Command)
			}
		} else {
			n.log = append(n.log, ent)
			n.ApplyConfig(ent.Command)
		}
	}
	if m.LeaderCommit > n.commitIndex {
		n.commitIndex = min(m.LeaderCommit, n.lastIndex())
	}
	match := n.lastIndex()
	if len(m.Entries) > 0 {
		match = m.Entries[len(m.Entries)-1].Index
	} else if m.PrevLogIndex > 0 {
		match = m.PrevLogIndex
	}
	effs := n.applyCommitted()
	effs = append(effs, n.send(m.From, peerMsg{
		Type: msgAppendEntriesResp, Term: n.currentTerm, From: n.ID,
		Success: true, MatchIndex: match, Durable: true,
	})...)
	return n.persistThen(effs)
}

func (n *Node) truncateFrom(index uint64) {
	out := n.log[:1]
	for _, e := range n.log[1:] {
		if e.Index < index {
			out = append(out, e)
		}
	}
	n.log = out
	n.rebuildConfigFromLog()
}

func (n *Node) rebuildConfigFromLog() {
	n.cfg = initConfig(n.bootstrap)
	n.Peers = append([]runtime.NodeID(nil), n.bootstrap...)
	n.jointIndex = 0
	for _, e := range n.log[1:] {
		n.ApplyConfig(e.Command)
		if len(n.cfg.JointWith) > 0 {
			n.jointIndex = e.Index
		} else {
			n.jointIndex = 0
		}
	}
}

func (n *Node) onAppendEntriesResp(m peerMsg) []runtime.Effect {
	if n.role != Leader || m.Term != n.currentTerm {
		return nil
	}
	if !m.Success {
		if n.nextIndex[m.From] > 1 {
			n.nextIndex[m.From]--
		}
		return n.sendAppend(m.From)
	}
	n.matchIndex[m.From] = m.MatchIndex
	n.nextIndex[m.From] = m.MatchIndex + 1
	old := n.commitIndex
	out := n.maybeCommit()
	if n.commitIndex > old {
		for _, p := range n.Peers {
			if p == n.ID {
				continue
			}
			out = append(out, n.sendAppend(p)...)
		}
	}
	return out
}

func (n *Node) maybeCommit() []runtime.Effect {
	for N := n.lastIndex(); N > n.commitIndex; N-- {
		e, ok := n.entry(N)
		if !ok {
			continue
		}
		if !n.commitAnyTerm && e.Term != n.currentTerm {
			continue
		}
		if n.quorumCommitted(N) {
			n.commitIndex = N
			break
		}
	}
	return n.applyCommitted()
}

func (n *Node) applyCommitted() []runtime.Effect {
	var out []runtime.Effect
	for n.lastApplied < n.commitIndex {
		n.lastApplied++
		e, ok := n.entry(n.lastApplied)
		if !ok {
			break
		}
		// Config already applied at append time; re-apply is idempotent.
		n.ApplyConfig(e.Command)
		out = append(out, runtime.Apply{Index: e.Index, Term: e.Term, Command: e.Command})
		for req, idx := range n.waiting {
			if idx == e.Index {
				out = append(out, runtime.Reply{RequestID: req, Payload: []byte(fmt.Sprintf("ok:%d", e.Index))})
				delete(n.waiting, req)
			}
		}
	}
	if !n.holdFinalize && n.role == Leader && n.jointIndex > 0 && n.commitIndex >= n.jointIndex && len(n.cfg.JointWith) > 0 {
		out = append(out, n.ProposeFinalize()...)
	}
	return out
}

func (n *Node) votingPeers() []runtime.NodeID {
	cfg := n.config()
	seen := map[runtime.NodeID]bool{}
	var out []runtime.NodeID
	for _, v := range cfg.Voters {
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	for _, v := range cfg.JointWith {
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out
}

// electionQuorum is majority of voters (both configs while joint).
func (n *Node) electionQuorum(votes map[runtime.NodeID]bool) bool {
	cfg := n.config()
	if len(cfg.Voters) == 0 {
		return false
	}
	if len(cfg.JointWith) > 0 {
		return majorityVotes(cfg.Voters, votes) && majorityVotes(cfg.JointWith, votes)
	}
	return majorityVotes(cfg.Voters, votes)
}

func majorityVotes(voters []runtime.NodeID, votes map[runtime.NodeID]bool) bool {
	if len(voters) == 0 {
		return false
	}
	count := 0
	for _, v := range voters {
		if votes[v] {
			count++
		}
	}
	return count*2 > len(voters)
}

func (n *Node) send(to runtime.NodeID, m peerMsg) []runtime.Effect {
	m.From = n.ID
	m.To = to
	b, _ := json.Marshal(m)
	return []runtime.Effect{runtime.Send{To: to, Payload: b}}
}

func (n *Node) persistThen(then []runtime.Effect) []runtime.Effect {
	hs := hardState{
		Term: n.currentTerm, VotedFor: n.votedFor, Log: n.log,
		SnapIdx: n.snapIndex, SnapTerm: n.snapTerm, Config: n.config(),
	}
	b, _ := json.Marshal(hs)
	n.persistID = fmt.Sprintf("p-%d-%d", n.ID, n.tickCount)
	n.syncInFlight = true
	n.afterSync = append(n.afterSync, then...)
	return []runtime.Effect{
		runtime.Persist{RequestID: n.persistID, Data: b},
		runtime.Sync{RequestID: n.persistID},
	}
}

func (n *Node) onDisk(d runtime.DiskCompletion) []runtime.Effect {
	if d.RequestID != n.persistID {
		return nil
	}
	if d.Err != nil || !d.Synced {
		return []runtime.Effect{runtime.Sync{RequestID: n.persistID}}
	}
	n.syncInFlight = false
	out := n.afterSync
	n.afterSync = nil
	for len(n.inbox) > 0 && !n.syncInFlight {
		cmd := n.inbox[0]
		n.inbox = n.inbox[1:]
		out = append(out, n.onClient(cmd)...)
	}
	return out
}

// RestoreHardState loads durable state after restart (from WAL payload).
func (n *Node) RestoreHardState(data []byte) error {
	var hs hardState
	if err := json.Unmarshal(data, &hs); err != nil {
		return err
	}
	n.currentTerm = hs.Term
	n.votedFor = hs.VotedFor
	if len(hs.Log) > 0 {
		n.log = hs.Log
	}
	n.snapIndex = hs.SnapIdx
	n.snapTerm = hs.SnapTerm
	if len(hs.Config.Voters) > 0 {
		n.cfg = hs.Config
		n.ApplyConfig(mustConfigCmd(hs.Config))
	}
	n.role = Follower
	n.commitIndex = n.snapIndex
	n.lastApplied = n.snapIndex
	return nil
}

func mustConfigCmd(cfg Config) []byte {
	body, _ := json.Marshal(cfg)
	cmd, _ := json.Marshal(map[string]any{"kind": "Config", "config": json.RawMessage(body)})
	return cmd
}

// CompactLog drops entries with index <= lastIncluded after a snapshot publish.
func (n *Node) CompactLog(lastIncluded, lastTerm uint64) {
	n.snapIndex = lastIncluded
	n.snapTerm = lastTerm
	out := []Entry{{Term: lastTerm, Index: lastIncluded}}
	for _, e := range n.log {
		if e.Index > lastIncluded {
			out = append(out, e)
		}
	}
	n.log = out
	if n.commitIndex < lastIncluded {
		n.commitIndex = lastIncluded
	}
	if n.lastApplied < lastIncluded {
		n.lastApplied = lastIncluded
	}
}

// TakeSnapshotData is set by the adapter via PendingSnap after InstallSnapshot.
func (n *Node) PendingSnap() []byte {
	b := n.pendingSnap
	n.pendingSnap = nil
	return b
}

func (n *Node) onInstallSnapshot(m peerMsg) []runtime.Effect {
	if m.Term < n.currentTerm {
		return n.send(m.From, peerMsg{Type: msgInstallSnapshotResp, Term: n.currentTerm, From: n.ID, Success: false})
	}
	if m.Term > n.currentTerm {
		n.becomeFollower(m.Term)
	}
	// Reject older snapshot (no apply regress).
	if m.SnapIndex < n.snapIndex || m.SnapIndex < n.lastApplied {
		return n.send(m.From, peerMsg{Type: msgInstallSnapshotResp, Term: n.currentTerm, From: n.ID, Success: false})
	}
	n.pendingSnap = append([]byte(nil), m.SnapData...)
	n.CompactLog(m.SnapIndex, m.SnapTerm)
	n.electionTicks = 0
	return n.persistThen(n.send(m.From, peerMsg{
		Type: msgInstallSnapshotResp, Term: n.currentTerm, From: n.ID, Success: true, MatchIndex: m.SnapIndex,
	}))
}

func (n *Node) onInstallSnapshotResp(m peerMsg) []runtime.Effect {
	if n.role != Leader || !m.Success {
		return nil
	}
	n.matchIndex[m.From] = m.MatchIndex
	n.nextIndex[m.From] = m.MatchIndex + 1
	return n.maybeCommit()
}

// SendInstallSnapshot queues an InstallSnapshot RPC (leader catch-up).
func (n *Node) SendInstallSnapshot(to runtime.NodeID, data []byte) []runtime.Effect {
	return n.send(to, peerMsg{
		Type: msgInstallSnapshot, Term: n.currentTerm, From: n.ID, To: to,
		SnapIndex: n.snapIndex, SnapTerm: n.snapTerm, SnapData: data,
	})
}

// ReadIndex returns a read-safe commit index for fixed configuration only.
// ok=false means caller must use the logged-fence path (joint or not leader).
func (n *Node) ReadIndex() (uint64, bool) {
	if n.role != Leader || len(n.config().JointWith) > 0 {
		return 0, false
	}
	if !n.quorumCommitted(n.commitIndex) {
		return 0, false
	}
	return n.commitIndex, true
}

// CommitIndex exposes commit for tests.
func (n *Node) CommitIndex() uint64 { return n.commitIndex }
func (n *Node) Role() Role          { return n.role }
func (n *Node) Term() uint64        { return n.currentTerm }
func (n *Node) LastApplied() uint64 { return n.lastApplied }
func (n *Node) SnapIndex() uint64   { return n.snapIndex }

// SetSnapData stores published snapshot bytes for InstallSnapshot catch-up.
func (n *Node) SetSnapData(data []byte) {
	n.snapData = append([]byte(nil), data...)
}

// LogLen returns number of retained entries excluding the snap placeholder.
func (n *Node) LogLen() int { return len(n.log) - 1 }

// LogEntries returns a copy of the retained log including the snap placeholder.
func (n *Node) LogEntries() []Entry {
	out := make([]Entry, len(n.log))
	copy(out, n.log)
	return out
}
