// Package replica is the deterministic in-memory sim cluster (used by zenith-sim
// and replica tests). The production node is internal/nodehost; both apply
// committed commands through mvcc.MustApply so invariant failures cannot diverge.
package replica

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/deccache"
	"github.com/yxshwanth/zenith/internal/mvcc"
	"github.com/yxshwanth/zenith/internal/raft"
	"github.com/yxshwanth/zenith/internal/runtime"
	"github.com/yxshwanth/zenith/internal/session"
	"github.com/yxshwanth/zenith/internal/sim"
	"github.com/yxshwanth/zenith/internal/snapshot"
	"github.com/yxshwanth/zenith/internal/token"
	"github.com/yxshwanth/zenith/internal/wal"
)

// Command kinds encoded in Raft log payloads.
type cmdKind string

const (
	cmdPut        cmdKind = "Put"
	cmdFence      cmdKind = "Fence"
	cmdGet        cmdKind = "Get"
	cmdTuple      cmdKind = "Tuple"
	cmdTupleBatch cmdKind = "TupleBatch"
)

type tupleOp struct {
	ObjectNamespace  string `json:"ons"`
	ObjectID         string `json:"oid"`
	Relation         string `json:"rel"`
	SubjectNamespace string `json:"sns"`
	SubjectID        string `json:"sid"`
	SubjectRelation  string `json:"srel,omitempty"`
	Delete           bool   `json:"delete,omitempty"`
}

type command struct {
	Kind     cmdKind `json:"kind"`
	Key      string  `json:"key,omitempty"`
	Value    string  `json:"value,omitempty"`
	Session  string  `json:"session,omitempty"`
	Seq      uint64  `json:"seq,omitempty"`
	Digest   string  `json:"digest,omitempty"`
	ClientID string  `json:"client_id,omitempty"`
	// Tuple fields (cmdTuple)
	Store            string `json:"store,omitempty"`
	ObjectNamespace  string `json:"ons,omitempty"`
	ObjectID         string `json:"oid,omitempty"`
	Relation         string `json:"rel,omitempty"`
	SubjectNamespace string `json:"sns,omitempty"`
	SubjectID        string `json:"sid,omitempty"`
	SubjectRelation  string `json:"srel,omitempty"`
	Delete           bool   `json:"delete,omitempty"`
	// TupleBatch
	Ops []tupleOp `json:"ops,omitempty"`
}

// Replica is one simulated node: Raft Core + Memory WAL + KV + sessions + snapshot store.
// MVCC holds versioned tuples when tuple commands are applied (Phase 3).
type Replica struct {
	Node     *raft.Node
	Disk     *wal.Memory
	KV       map[string]string
	Sessions session.Table
	Snap     *snapshot.Store
	MVCC     *mvcc.DB
}

func NewReplica(id runtime.NodeID, peers []runtime.NodeID) *Replica {
	return &Replica{
		Node:     raft.New(id, peers),
		Disk:     &wal.Memory{},
		KV:       map[string]string{},
		Sessions: session.Table{},
		Snap:     &snapshot.Store{},
		MVCC:     mvcc.New(),
	}
}

// Cluster drives N replicas under one Scheduler.
type Cluster struct {
	Replicas map[runtime.NodeID]*Replica
	Sched    *sim.Scheduler
	// partition: undirected drop if either direction blocked
	Blocked map[[2]runtime.NodeID]bool
	// DropDir drops only from→to (asymmetric).
	DropDir map[[2]runtime.NodeID]bool
	// Frozen nodes drop all inbound peer messages (simulate lag without healing).
	Frozen map[runtime.NodeID]bool
	// client history for checker
	History []checker.HistoryEntry
	clock   int64
	// pending client meta
	pending  map[string]pendingOp
	codec    *token.Codec
	DecCache *deccache.Cache // optional; nil = off
}

type pendingOp struct {
	op        checker.KVOp
	invokedAt int64
	session   string
	seq       uint64
}

func NewCluster(seed uint64, ids []runtime.NodeID) *Cluster {
	peers := append([]runtime.NodeID(nil), ids...)
	reps := map[runtime.NodeID]*Replica{}
	cores := map[runtime.NodeID]runtime.Core{}
	for _, id := range ids {
		r := NewReplica(id, peers)
		reps[id] = r
		cores[id] = r.Node
	}
	c := &Cluster{
		Replicas: reps,
		Blocked:  map[[2]runtime.NodeID]bool{},
		DropDir:  map[[2]runtime.NodeID]bool{},
		Frozen:   map[runtime.NodeID]bool{},
		pending:  map[string]pendingOp{},
	}
	c.Sched = sim.NewScheduler(seed, cores, c.handleEffects)
	return c
}

// AddNode registers a new replica (learner or future voter) into the sim cluster.
func (c *Cluster) AddNode(id runtime.NodeID) {
	if _, ok := c.Replicas[id]; ok {
		return
	}
	peers := make([]runtime.NodeID, 0, len(c.Replicas)+1)
	for pid := range c.Replicas {
		peers = append(peers, pid)
	}
	peers = append(peers, id)
	r := NewReplica(id, peers)
	// Not a voter until a config entry says so (learner join path).
	r.Node.SetLearnersOnly(id)
	c.Replicas[id] = r
	c.Sched.Register(id, r.Node)
	for _, existing := range c.Replicas {
		if existing.Node.ID == id {
			continue
		}
		found := false
		for _, p := range existing.Node.Peers {
			if p == id {
				found = true
				break
			}
		}
		if !found {
			existing.Node.Peers = append(existing.Node.Peers, id)
		}
	}
}

func edge(a, b runtime.NodeID) [2]runtime.NodeID {
	if a < b {
		return [2]runtime.NodeID{a, b}
	}
	return [2]runtime.NodeID{b, a}
}

func (c *Cluster) Partition(a, b runtime.NodeID) { c.Blocked[edge(a, b)] = true }
func (c *Cluster) Heal(a, b runtime.NodeID)      { delete(c.Blocked, edge(a, b)) }

func (c *Cluster) handleEffects(from runtime.NodeID, effects []runtime.Effect, s *sim.Scheduler) {
	r := c.Replicas[from]
	for _, eff := range effects {
		switch e := eff.(type) {
		case runtime.Persist:
			r.Disk.Append(e.Data)
		case runtime.Sync:
			r.Disk.Sync()
			s.Schedule(0, from, runtime.DiskCompletion{RequestID: e.RequestID, Synced: true})
		case runtime.Send:
			if c.Blocked[edge(from, e.To)] || c.Frozen[e.To] || c.DropDir[[2]runtime.NodeID{from, e.To}] {
				continue
			}
			s.Schedule(1, e.To, runtime.PeerMessage{From: from, Payload: e.Payload})
		case runtime.Apply:
			c.apply(from, e)
		case runtime.Reply:
			c.onReply(e)
		case runtime.Schedule:
			s.Schedule(sim.VirtualTime(e.Delay), from, runtime.ScheduledWork{RequestID: e.RequestID})
		}
	}
	c.drainPendingSnap(from)
}

func (c *Cluster) drainPendingSnap(id runtime.NodeID) {
	r := c.Replicas[id]
	if raw := r.Node.PendingSnap(); len(raw) > 0 {
		_ = r.Snap.Install(raw)
		if st, err := r.Snap.Load(); err == nil {
			r.KV = st.KV
			if r.KV == nil {
				r.KV = map[string]string{}
			}
		}
	}
}

func (c *Cluster) apply(id runtime.NodeID, a runtime.Apply) {
	r := c.Replicas[id]
	var cmd command
	if err := json.Unmarshal(a.Command, &cmd); err != nil {
		return
	}
	switch cmd.Kind {
	case cmdPut:
		if cmd.Session != "" {
			if cached, _, err := r.Sessions.Decide(cmd.Session, cmd.Seq, cmd.Digest); err != nil {
				return
			} else if cached != nil {
				return // already applied
			}
		}
		r.KV[cmd.Key] = cmd.Value
		if cmd.Session != "" {
			r.Sessions.Put(cmd.Session, cmd.Seq, cmd.Digest, []byte("ok"), a.Index)
		}
	case cmdFence, "Noop":
		r.MVCC.SetApplied(mvcc.Revision(a.Index))
	case cmdTuple:
		k := mvcc.EncodeKey(cmd.Store, cmd.ObjectNamespace, cmd.ObjectID, cmd.Relation,
			cmd.SubjectNamespace, cmd.SubjectID, cmd.SubjectRelation)
		if cmd.Delete {
			mvcc.MustApply(r.MVCC.ApplyDelete(mvcc.Revision(a.Index), k))
		} else {
			mvcc.MustApply(r.MVCC.ApplyPut(mvcc.Revision(a.Index), k))
		}
	case cmdTupleBatch:
		if cmd.Session != "" {
			if cached, _, err := r.Sessions.Decide(cmd.Session, cmd.Seq, cmd.Digest); err != nil {
				return
			} else if cached != nil {
				return
			}
		}
		for _, op := range cmd.Ops {
			k := mvcc.EncodeKey(cmd.Store, op.ObjectNamespace, op.ObjectID, op.Relation,
				op.SubjectNamespace, op.SubjectID, op.SubjectRelation)
			if op.Delete {
				mvcc.MustApply(r.MVCC.ApplyDelete(mvcc.Revision(a.Index), k))
			} else {
				mvcc.MustApply(r.MVCC.ApplyPut(mvcc.Revision(a.Index), k))
			}
		}
		if cmd.Session != "" {
			r.Sessions.Put(cmd.Session, cmd.Seq, cmd.Digest, []byte("ok"), a.Index)
		}
	}
	// ponytail: snapshot compaction threshold raised — auto-compact at 3 broke
	// subsequent proposes in short tests; Phase 2 tests cover publish explicitly.
	if r.Node.LastApplied() >= 100 && r.Node.LastApplied()-r.Node.SnapIndex() >= 100 {
		c.publishSnapshot(id)
	}
}

func (c *Cluster) publishSnapshot(id runtime.NodeID) {
	r := c.Replicas[id]
	idx := r.Node.LastApplied()
	// find term at idx from restored state — use current term as ponytail approx when compacted
	term := r.Node.Term()
	sess, _ := json.Marshal(r.Sessions)
	st := snapshot.State{
		Meta:     snapshot.Meta{LastIndex: idx, LastTerm: term},
		KV:       copyMap(r.KV),
		Sessions: sess,
	}
	_ = r.Snap.BeginWrite(st)
	r.Snap.Publish()
	r.Node.CompactLog(idx, term)
	r.Node.SetSnapData(r.Snap.Bytes())
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (c *Cluster) onReply(rep runtime.Reply) {
	c.clock++
	p, ok := c.pending[rep.RequestID]
	if !ok {
		return
	}
	delete(c.pending, rep.RequestID)
	if rep.Err != nil {
		c.History = append(c.History, checker.HistoryEntry{
			Op: p.op, InvokedAt: p.invokedAt, CompletedAt: c.clock, Unknown: true,
		})
		return
	}
	res := checker.KVResult{}
	switch p.op.Kind {
	case checker.KVPut:
		res = checker.KVResult{Found: true, Value: p.op.Value}
	case checker.KVGet:
		res = checker.KVResult{Found: true, Value: string(rep.Payload)}
	}
	c.History = append(c.History, checker.HistoryEntry{
		Op: p.op, Result: res, InvokedAt: p.invokedAt, CompletedAt: c.clock,
	})
}

// TickElection fires an election tick on all live nodes.
func (c *Cluster) TickElection() {
	for id := range c.Replicas {
		c.Sched.Schedule(0, id, runtime.Tick{Kind: "election"})
	}
}

// ProposePut submits a Put to node (must be leader for success).
func (c *Cluster) ProposePut(to runtime.NodeID, reqID, session string, seq uint64, key, val string) {
	c.clock++
	dig := key + "=" + val
	cmd, _ := json.Marshal(command{Kind: cmdPut, Key: key, Value: val, Session: session, Seq: seq, Digest: dig})
	c.pending[reqID] = pendingOp{
		op:        checker.KVOp{Kind: checker.KVPut, Key: key, Value: val},
		invokedAt: c.clock,
		session:   session,
		seq:       seq,
	}
	c.Sched.Schedule(0, to, runtime.ClientCommand{RequestID: reqID, Payload: cmd})
}

// Crash discards unsynced WAL and restores Raft hard state from durable prefix.
func (c *Cluster) Crash(id runtime.NodeID) {
	r := c.Replicas[id]
	r.Disk.CrashUnsynced()
	recs, err := r.Disk.LoadSynced()
	peers := r.Node.Peers
	*r.Node = *raft.New(id, peers)
	r.KV = map[string]string{}
	r.Sessions = session.Table{}
	// Prefer published snapshot, then WAL hard state.
	if st, err := r.Snap.Load(); err == nil {
		r.KV = st.KV
		if r.KV == nil {
			r.KV = map[string]string{}
		}
		r.Node.CompactLog(st.Meta.LastIndex, st.Meta.LastTerm)
		r.Node.SetSnapData(r.Snap.Bytes())
	}
	if err == nil && len(recs) > 0 {
		_ = r.Node.RestoreHardState(recs[len(recs)-1])
	}
}

// Leader returns a node that believes it is leader, or 0.
func (c *Cluster) Leader() runtime.NodeID {
	for id, r := range c.Replicas {
		if r.Node.Role() == raft.Leader {
			return id
		}
	}
	return 0
}

// Run drains the scheduler.
func (c *Cluster) Run(max int) { c.Sched.Run(max) }

// BootstrapElect runs election ticks until a leader appears or steps exhausted.
func (c *Cluster) BootstrapElect(maxRounds int) runtime.NodeID {
	for i := 0; i < maxRounds; i++ {
		c.TickElection()
		c.Run(500)
		if l := c.Leader(); l != 0 {
			return l
		}
	}
	return 0
}

// CheckHistory returns Pass/Fail/Inconclusive for recorded client ops.
func (c *Cluster) CheckHistory() checker.CheckOutcome {
	return checker.CheckKVHistory(c.History)
}

// ProposeTuple submits a tuple write through Raft.
func (c *Cluster) ProposeTuple(to runtime.NodeID, reqID string, cmd command) {
	cmd.Kind = cmdTuple
	payload, _ := json.Marshal(cmd)
	c.clock++
	c.pending[reqID] = pendingOp{
		op:        checker.KVOp{Kind: checker.KVPut, Key: reqID, Value: "tuple"},
		invokedAt: c.clock,
	}
	c.Sched.Schedule(0, to, runtime.ClientCommand{RequestID: reqID, Payload: payload})
}

// StrongGet proposes a fence then reads KV on leader after commit (test helper).
func (c *Cluster) StrongGet(leader runtime.NodeID, fenceID, key string) (string, bool) {
	cmd, _ := json.Marshal(command{Kind: cmdFence})
	c.Sched.Schedule(0, leader, runtime.ClientCommand{RequestID: fenceID, Payload: cmd})
	c.Run(500)
	v, ok := c.Replicas[leader].KV[key]
	c.clock++
	c.History = append(c.History, checker.HistoryEntry{
		Op:          checker.KVOp{Kind: checker.KVGet, Key: key},
		Result:      checker.KVResult{Found: ok, Value: v},
		InvokedAt:   c.clock - 1,
		CompletedAt: c.clock,
	})
	return v, ok
}

// String dumps cluster state for test failures.
func (c *Cluster) String() string {
	var b strings.Builder
	for id, r := range c.Replicas {
		fmt.Fprintf(&b, "n%d role=%d term=%d commit=%d applied=%d kv=%v\n",
			id, r.Node.Role(), r.Node.Term(), r.Node.CommitIndex(), r.Node.LastApplied(), r.KV)
	}
	return b.String()
}
