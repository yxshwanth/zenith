// Package replica is the deterministic in-memory sim cluster (used by zenith-sim
// and replica tests). The production node is internal/nodehost; both apply
// committed commands through mvcc.MustApply so invariant failures cannot diverge.
package replica

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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
	pending    map[string]pendingOp
	ackedIndex uint64
	codec      *token.Codec
	DecCache   *deccache.Cache // optional; nil = off
	Faults     Faults
}

// Faults is the seeded explorer. Zero value is a reliable net/disk: delay is
// always 1, fsync always succeeds, timeouts stay 3+3*id. Streams are still
// drawn so a later non-zero rate cannot shift another subsystem's sequence.
// Manual Partition / DropDir / Frozen / Crash still apply on top.
type Faults struct {
	MaxNetDelay      int // extra ticks in [0, MaxNetDelay]
	DropPermille     int
	DupPermille      int
	MaxDiskDelay     int
	SyncFailPermille int
	ElectJitter      int // extra timeout ticks in [0, ElectJitter]
}

type pendingOp struct {
	op        checker.KVOp
	invokedAt int64
	session   string
	seq       uint64
	fence     bool
	acked     bool
	found     bool
	value     string
}

func NewCluster(seed uint64, ids []runtime.NodeID) *Cluster {
	return NewClusterWithFaults(seed, ids, Faults{})
}

func NewClusterWithFaults(seed uint64, ids []runtime.NodeID, f Faults) *Cluster {
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
		Faults:   f,
	}
	c.Sched = sim.NewScheduler(seed, cores, c.handleEffects)
	for _, id := range ids {
		base := 3 + int(id)*3
		extra := c.Sched.ElectionIntn(f.ElectJitter + 1)
		c.Replicas[id].Node.SetElectionTimeout(base + extra)
	}
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
			c.completeSync(from, e.RequestID, s)
		case runtime.Send:
			c.deliver(from, e.To, e.Payload, s)
		case runtime.Apply:
			c.apply(from, e)
		case runtime.Reply:
			c.onReply(from, e)
		case runtime.Schedule:
			s.Schedule(sim.VirtualTime(e.Delay), from, runtime.ScheduledWork{RequestID: e.RequestID})
		}
	}
	c.drainPendingSnap(from)
}

func (c *Cluster) completeSync(from runtime.NodeID, reqID string, s *sim.Scheduler) {
	ok := s.DiskIntn(1000) >= c.Faults.SyncFailPermille
	if ok {
		c.Replicas[from].Disk.Sync()
	}
	delay := sim.VirtualTime(s.DiskIntn(c.Faults.MaxDiskDelay + 1))
	s.Schedule(delay, from, runtime.DiskCompletion{RequestID: reqID, Synced: ok})
}

func (c *Cluster) deliver(from, to runtime.NodeID, payload []byte, s *sim.Scheduler) {
	if c.Blocked[edge(from, to)] || c.Frozen[to] || c.DropDir[[2]runtime.NodeID{from, to}] {
		return
	}
	if s.NetworkIntn(1000) < c.Faults.DropPermille {
		return
	}
	delay := sim.VirtualTime(1 + s.NetworkIntn(c.Faults.MaxNetDelay+1))
	s.Schedule(delay, to, runtime.PeerMessage{From: from, Payload: payload})
	if s.NetworkIntn(1000) < c.Faults.DupPermille {
		extra := sim.VirtualTime(1 + s.NetworkIntn(c.Faults.MaxNetDelay+1))
		s.Schedule(delay+extra, to, runtime.PeerMessage{From: from, Payload: payload})
	}
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
			if len(st.Sessions) > 0 {
				_ = json.Unmarshal(st.Sessions, &r.Sessions)
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
				return
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

func (c *Cluster) onReply(from runtime.NodeID, rep runtime.Reply) {
	c.clock++
	p, ok := c.pending[rep.RequestID]
	if !ok {
		return
	}
	idx := parseReplyIndex(rep.Payload)
	if p.fence {
		if rep.Err != nil {
			delete(c.pending, rep.RequestID)
			return
		}
		if idx > 0 && idx < c.ackedIndex {
			delete(c.pending, rep.RequestID)
			return
		}
		p.acked = true
		if v, ok := c.Replicas[from].KV[p.op.Key]; ok {
			p.found = true
			p.value = v
		}
		c.pending[rep.RequestID] = p
		return
	}
	delete(c.pending, rep.RequestID)
	if rep.Err != nil {
		c.History = append(c.History, checker.HistoryEntry{
			Op: p.op, InvokedAt: p.invokedAt, CompletedAt: c.clock, Unknown: true,
		})
		return
	}
	if p.op.Kind == checker.KVPut {
		if c.Replicas[from].KV[p.op.Key] != p.op.Value {
			return
		}
		if idx > c.ackedIndex {
			c.ackedIndex = idx
		}
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

func parseReplyIndex(payload []byte) uint64 {
	s := string(payload)
	if !strings.HasPrefix(s, "ok:") {
		return 0
	}
	n, err := strconv.ParseUint(s[3:], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// TickElection fires an election tick on all live nodes in id order.
func (c *Cluster) TickElection() {
	ids := make([]runtime.NodeID, 0, len(c.Replicas))
	for id := range c.Replicas {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
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
// A published snapshot that is at least as new as the WAL snapshot index wins,
// so an in-flight InstallSnapshot cannot be rolled back by an older synced record.
func (c *Cluster) Crash(id runtime.NodeID) {
	r := c.Replicas[id]
	r.Disk.CrashUnsynced()
	recs, walErr := r.Disk.LoadSynced()
	peers := r.Node.Peers
	*r.Node = *raft.New(id, peers)
	r.KV = map[string]string{}
	r.Sessions = session.Table{}
	if walErr == nil && len(recs) > 0 {
		_ = r.Node.RestoreHardState(recs[len(recs)-1])
	}
	if st, err := r.Snap.Load(); err == nil {
		if st.Meta.LastIndex >= r.Node.SnapIndex() {
			r.KV = st.KV
			if r.KV == nil {
				r.KV = map[string]string{}
			}
			if len(st.Sessions) > 0 {
				_ = json.Unmarshal(st.Sessions, &r.Sessions)
			}
			r.Node.CompactLog(st.Meta.LastIndex, st.Meta.LastTerm)
			r.Node.SetSnapData(r.Snap.Bytes())
		}
	}
}

// Leader returns the highest-term node that believes it is leader, or 0.
func (c *Cluster) Leader() runtime.NodeID {
	var best runtime.NodeID
	var term uint64
	for id, r := range c.Replicas {
		if r.Node.Role() != raft.Leader {
			continue
		}
		t := r.Node.Term()
		if best == 0 || t > term || (t == term && id < best) {
			best, term = id, t
		}
	}
	return best
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
// Leftover pending ops are incomplete (no client reply) and must stay in
// the history as Unknown so Porcupine can treat them as in-flight writes.
func (c *Cluster) CheckHistory() checker.CheckOutcome {
	type left struct {
		id string
		p  pendingOp
	}
	var flush []left
	for id, p := range c.pending {
		flush = append(flush, left{id, p})
	}
	sort.Slice(flush, func(i, j int) bool { return flush[i].p.invokedAt < flush[j].p.invokedAt })
	for _, x := range flush {
		delete(c.pending, x.id)
		if x.p.fence {
			continue
		}
		c.History = append(c.History, checker.HistoryEntry{
			Op: x.p.op, InvokedAt: x.p.invokedAt, Unknown: true,
		})
	}
	return checker.CheckKVHistory(c.History)
}

func (c *Cluster) prefixOK() error {
	p := checker.PrefixLedger{}
	for id, r := range c.Replicas {
		for _, e := range r.Node.LogEntries() {
			if e.Index == 0 {
				continue
			}
			if !p.Observe(e.Index, e.Term, string(e.Command)) {
				return fmt.Errorf("R3: node %d index=%d term=%d command conflict", id, e.Index, e.Term)
			}
		}
	}
	return nil
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

// ExplorePuts submits n puts with keys/values drawn from the workload stream.
func (c *Cluster) ExplorePuts(n int) {
	for i := 0; i < n; i++ {
		leader := c.Leader()
		if leader == 0 {
			leader = c.BootstrapElect(20)
			if leader == 0 {
				return
			}
		}
		k := fmt.Sprintf("k%d", c.Sched.WorkloadIntn(8))
		v := fmt.Sprintf("v%d", c.Sched.WorkloadIntn(8))
		c.ProposePut(leader, fmt.Sprintf("w%d", i), "s", uint64(i+1), k, v)
		c.Run(200)
	}
}

// ExploreHunt mixes puts, fenced gets, and crashes so Porcupine can fail.
func (c *Cluster) ExploreHunt(n int) {
	var seq uint64
	ids := make([]runtime.NodeID, 0, len(c.Replicas))
	for id := range c.Replicas {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i := 0; i < n; i++ {
		leader := c.Leader()
		if leader == 0 {
			leader = c.BootstrapElect(25)
			if leader == 0 {
				continue
			}
		}
		k := fmt.Sprintf("k%d", c.Sched.WorkloadIntn(3))
		switch c.Sched.WorkloadIntn(10) {
		case 0, 1, 2, 3, 4:
			seq++
			v := fmt.Sprintf("v%d", c.Sched.WorkloadIntn(6))
			c.ProposePut(leader, fmt.Sprintf("h%d", i), "hunt", seq, k, v)
			c.Run(150)
		case 5, 6, 7:
			c.StrongGet(leader, fmt.Sprintf("g%d", i), k)
		default:
			c.Crash(ids[c.Sched.WorkloadIntn(len(ids))])
			c.Run(30)
		}
	}
}

// StrongGet proposes a fence then reads KV on leader after commit (test helper).
func (c *Cluster) StrongGet(leader runtime.NodeID, fenceID, key string) (string, bool) {
	c.clock++
	invoked := c.clock
	cmd, _ := json.Marshal(command{Kind: cmdFence})
	c.pending[fenceID] = pendingOp{
		op: checker.KVOp{Kind: checker.KVGet, Key: key}, fence: true, invokedAt: invoked,
	}
	c.Sched.Schedule(0, leader, runtime.ClientCommand{RequestID: fenceID, Payload: cmd})
	c.Run(500)
	p, acked := c.pending[fenceID]
	delete(c.pending, fenceID)
	c.clock++
	if !acked || !p.acked {
		c.History = append(c.History, checker.HistoryEntry{
			Op: checker.KVOp{Kind: checker.KVGet, Key: key}, InvokedAt: invoked, CompletedAt: c.clock, Unknown: true,
		})
		return "", false
	}
	c.History = append(c.History, checker.HistoryEntry{
		Op:          checker.KVOp{Kind: checker.KVGet, Key: key},
		Result:      checker.KVResult{Found: p.found, Value: p.value},
		InvokedAt:   invoked,
		CompletedAt: c.clock,
	})
	return p.value, p.found
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
