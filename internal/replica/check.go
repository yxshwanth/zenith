package replica

import (
	"encoding/json"
	"fmt"

	v2 "github.com/yxshwanth/zenith/api/zenith/v2"
	"github.com/yxshwanth/zenith/internal/authz"
	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/coordinator"
	"github.com/yxshwanth/zenith/internal/deccache"
	"github.com/yxshwanth/zenith/internal/mvcc"
	"github.com/yxshwanth/zenith/internal/runtime"
	"github.com/yxshwanth/zenith/internal/token"
)

// TokenCodec returns the cluster token codec.
func (c *Cluster) TokenCodec() *token.Codec {
	if c.codec == nil {
		c.codec = &token.Codec{Secret: []byte("zenith-dev"), Epoch: 1, Group: 1}
	}
	return c.codec
}

// CheckPermission evaluates authz at a selected revision on node id.
func (c *Cluster) CheckPermission(id runtime.NodeID, req v2.CheckRequest) v2.CheckResponse {
	codec := c.TokenCodec()
	if req.RequiredToken != "" {
		if _, err := codec.Verify(req.RequiredToken, req.Store); err != nil {
			return v2.CheckResponse{Err: err}
		}
	}
	req.Consistency.Normalize()
	r := c.Replicas[id]
	if req.Consistency.Mode == v2.FullyConsistent {
		if idx, ok := r.Node.ReadIndex(); ok {
			r.MVCC.SetApplied(mvcc.Revision(idx))
			if r.MVCC.Applied() < mvcc.Revision(r.Node.LastApplied()) {
				r.MVCC.SetApplied(mvcc.Revision(r.Node.LastApplied()))
			}
		} else {
			c.StrongGet(id, "fence-"+req.SubjectID, "__fence__")
			r.MVCC.SetApplied(mvcc.Revision(r.Node.LastApplied()))
		}
	}
	var tokClaims *token.Claims
	if req.RequiredToken != "" {
		cl, err := codec.Verify(req.RequiredToken, req.Store)
		if err != nil {
			return v2.CheckResponse{Err: err}
		}
		tokClaims = &cl
	}
	st := coordinator.State{Applied: r.MVCC.Applied(), Floor: 0}
	if tokClaims != nil && st.Applied < mvcc.Revision(tokClaims.Revision) {
		return v2.CheckResponse{Err: fmt.Errorf("replica: behind required revision %d (applied %d)", tokClaims.Revision, st.Applied)}
	}
	rev, err := coordinator.SelectRevision(req.Consistency, st, tokClaims)
	if err != nil {
		return v2.CheckResponse{Err: err}
	}
	if tokClaims != nil && rev < mvcc.Revision(tokClaims.Revision) {
		return v2.CheckResponse{Err: fmt.Errorf("replica: cannot satisfy required token rev")}
	}
	snap, err := r.MVCC.Open(rev)
	if err != nil {
		return v2.CheckResponse{Err: err}
	}
	defer snap.Close()
	ck := deccache.Key{
		Store: req.Store, ONS: req.ObjectNamespace, OID: req.ObjectID, Rel: req.Relation,
		SNS: req.SubjectNamespace, SID: req.SubjectID, Revision: uint64(rev),
	}
	if c.DecCache != nil {
		if e, ok := c.DecCache.Get(ck); ok {
			return v2.CheckResponse{Allowed: e.Allowed, Token: codec.Mint(req.Store, uint64(rev))}
		}
	}
	eng := &authz.Engine{Snap: snap}
	res, err := eng.Check(req.Store, req.ObjectNamespace, req.ObjectID, req.Relation, req.SubjectNamespace, req.SubjectID)
	if err != nil || res == authz.Incomplete {
		return v2.CheckResponse{Err: fmt.Errorf("incomplete: %v", err)}
	}
	allowed := res == authz.Allow
	if c.DecCache != nil {
		c.DecCache.Put(ck, allowed)
	}
	return v2.CheckResponse{Allowed: allowed, Token: codec.Mint(req.Store, uint64(rev))}
}

// CheckContentChange commits a fence and evaluates writer permission at that revision.
func (c *Cluster) CheckContentChange(leader runtime.NodeID, req v2.CheckContentChangeRequest) v2.CheckContentChangeResponse {
	c.StrongGet(leader, req.RequestID+"-fence", "__ccc__")
	r := c.Replicas[leader]
	r.MVCC.SetApplied(mvcc.Revision(r.Node.LastApplied()))
	rev := r.MVCC.Applied()
	if rev == 0 {
		return v2.CheckContentChangeResponse{Err: fmt.Errorf("no applied revision")}
	}
	snap, err := r.MVCC.Open(rev)
	if err != nil {
		return v2.CheckContentChangeResponse{Err: err}
	}
	defer snap.Close()
	eng := &authz.Engine{Snap: snap}
	res, err := eng.Check(req.Store, req.ObjectNamespace, req.ObjectID, req.Relation, req.SubjectNamespace, req.SubjectID)
	if err != nil || res == authz.Incomplete {
		return v2.CheckContentChangeResponse{Err: fmt.Errorf("incomplete")}
	}
	if res != authz.Allow {
		return v2.CheckContentChangeResponse{Allowed: false, Rev: uint64(rev)}
	}
	tok := c.TokenCodec().Mint(req.Store, uint64(rev))
	return v2.CheckContentChangeResponse{Allowed: true, Token: tok, Rev: uint64(rev)}
}

// BatchCheck evaluates all items at one selected snapshot (A4).
func (c *Cluster) BatchCheck(id runtime.NodeID, req v2.BatchCheckRequest) v2.BatchCheckResponse {
	req.Consistency.Normalize()
	r := c.Replicas[id]
	if req.Consistency.Mode == v2.FullyConsistent {
		c.StrongGet(id, "batch-fence", "__batch__")
		r.MVCC.SetApplied(mvcc.Revision(r.Node.LastApplied()))
	}
	st := coordinator.State{Applied: r.MVCC.Applied(), Floor: 0}
	rev, err := coordinator.SelectRevision(req.Consistency, st, nil)
	if err != nil {
		return v2.BatchCheckResponse{Err: err}
	}
	snap, err := r.MVCC.Open(rev)
	if err != nil {
		return v2.BatchCheckResponse{Err: err}
	}
	defer snap.Close()
	codec := c.TokenCodec()
	out := v2.BatchCheckResponse{
		Revision: uint64(rev),
		Token:    codec.Mint(req.Store, uint64(rev)),
		Results:  make([]v2.CheckResponse, len(req.Items)),
	}
	eng := &authz.Engine{Snap: snap}
	for i, item := range req.Items {
		store := item.Store
		if store == "" {
			store = req.Store
		}
		// A4: every item uses the same snap revision — ignore per-item Consistency.
		if snap.Revision() != rev {
			out.Results[i] = v2.CheckResponse{Err: fmt.Errorf("batch: revision drift")}
			out.Err = fmt.Errorf("batch: revision drift")
			continue
		}
		res, err := eng.Check(store, item.ObjectNamespace, item.ObjectID, item.Relation, item.SubjectNamespace, item.SubjectID)
		if err != nil || res == authz.Incomplete {
			out.Results[i] = v2.CheckResponse{Err: fmt.Errorf("incomplete: %v", err)}
			continue
		}
		out.Results[i] = v2.CheckResponse{Allowed: res == authz.Allow, Token: out.Token}
	}
	return out
}

// WriteTuples proposes an atomic tuple batch through Raft on the leader.
func (c *Cluster) WriteTuples(leader runtime.NodeID, req v2.WriteTuplesRequest) v2.WriteTuplesResponse {
	ops := make([]tupleOp, 0, len(req.Tuples)+len(req.Deletes))
	for _, t := range req.Tuples {
		ops = append(ops, tupleOp{
			ObjectNamespace: t.ObjectNamespace, ObjectID: t.ObjectID, Relation: t.Relation,
			SubjectNamespace: t.SubjectNamespace, SubjectID: t.SubjectID, SubjectRelation: t.SubjectRelation,
		})
	}
	for _, t := range req.Deletes {
		ops = append(ops, tupleOp{
			ObjectNamespace: t.ObjectNamespace, ObjectID: t.ObjectID, Relation: t.Relation,
			SubjectNamespace: t.SubjectNamespace, SubjectID: t.SubjectID, SubjectRelation: t.SubjectRelation,
			Delete: true,
		})
	}
	dig := fmt.Sprintf("%s:%d:%d", req.Store, req.Seq, len(ops))
	payload, _ := json.Marshal(command{
		Kind: cmdTupleBatch, Store: req.Store, Session: req.Session, Seq: req.Seq, Digest: dig, Ops: ops,
	})
	reqID := req.RequestID
	if reqID == "" {
		reqID = dig
	}
	c.clock++
	c.pending[reqID] = pendingOp{op: checker.KVOp{Kind: checker.KVPut, Key: reqID, Value: "batch"}, invokedAt: c.clock}
	c.Sched.Schedule(0, leader, runtime.ClientCommand{RequestID: reqID, Payload: payload})
	c.Run(2000)
	r := c.Replicas[leader]
	rev := uint64(r.MVCC.Applied())
	if rev == 0 {
		return v2.WriteTuplesResponse{Err: fmt.Errorf("write: not applied")}
	}
	return v2.WriteTuplesResponse{Token: c.TokenCodec().Mint(req.Store, rev), Rev: rev}
}

// ListSubjects enumerates subjects at one selected revision.
func (c *Cluster) ListSubjects(id runtime.NodeID, req v2.ListSubjectsRequest) v2.ListSubjectsResponse {
	req.Consistency.Normalize()
	r := c.Replicas[id]
	if req.Consistency.Mode == v2.FullyConsistent {
		c.StrongGet(id, "ls-fence", "__ls__")
		r.MVCC.SetApplied(mvcc.Revision(r.Node.LastApplied()))
	}
	st := coordinator.State{Applied: r.MVCC.Applied(), Floor: 0}
	rev, err := coordinator.SelectRevision(req.Consistency, st, nil)
	if err != nil {
		return v2.ListSubjectsResponse{Err: err}
	}
	snap, err := r.MVCC.Open(rev)
	if err != nil {
		return v2.ListSubjectsResponse{Err: err}
	}
	defer snap.Close()
	eng := &authz.Engine{Snap: snap}
	subs, err := eng.ListSubjects(req.Store, req.ObjectNamespace, req.ObjectID, req.Relation, req.Limit)
	if err != nil {
		return v2.ListSubjectsResponse{Err: err, Rev: uint64(rev)}
	}
	out := make([]v2.Subject, len(subs))
	for i, s := range subs {
		out[i] = v2.Subject{SubjectNamespace: s.SubjectNamespace, SubjectID: s.SubjectID, SubjectRelation: s.SubjectRelation}
	}
	return v2.ListSubjectsResponse{
		Subjects: out, Token: c.TokenCodec().Mint(req.Store, uint64(rev)), Rev: uint64(rev), Complete: true,
	}
}

// GetRequestOutcome looks up a sessioned write after a fence.
func (c *Cluster) GetRequestOutcome(leader runtime.NodeID, req v2.GetRequestOutcomeRequest) v2.GetRequestOutcomeResponse {
	c.StrongGet(leader, req.RequestID+"-out", "__out__")
	rec, ok := c.Replicas[leader].Sessions.Lookup(req.Session, req.Seq)
	if !ok {
		return v2.GetRequestOutcomeResponse{Found: false}
	}
	return v2.GetRequestOutcomeResponse{Found: true, Rev: rec.Revision, Result: string(rec.Result)}
}
