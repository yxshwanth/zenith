// Package nodehost is the real-process Raft+MVCC host shared by zenithd.
//
// ponytail: single-threaded effect apply under one mutex; JSON peer framing.
package nodehost

import (
	"crypto/hmac"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yxshwanth/zenith/internal/authz"
	"github.com/yxshwanth/zenith/internal/mvcc"
	"github.com/yxshwanth/zenith/internal/raft"
	"github.com/yxshwanth/zenith/internal/runtime"
	"github.com/yxshwanth/zenith/internal/session"
	"github.com/yxshwanth/zenith/internal/snapshot"
	"github.com/yxshwanth/zenith/internal/token"
	"github.com/yxshwanth/zenith/internal/wal"
)

// Host is one real node.
type Host struct {
	mu        sync.RWMutex
	ID        runtime.NodeID
	Node      *raft.Node
	Disk      *wal.File
	MVCC      *mvcc.DB
	Sessions  session.Table
	KV        map[string]string
	PeerAddrs map[runtime.NodeID]string
	Codec     *token.Codec
	Snap      *snapshot.Store
	AuthToken string
	TLS       *tls.Config // nil = plaintext
	waiting   map[string]chan struct{}
}

func Open(id runtime.NodeID, peers []runtime.NodeID, peerAddrs map[runtime.NodeID]string, dataDir string, tokenSecret []byte) (*Host, error) {
	disk, err := wal.OpenFile(dataDir)
	if err != nil {
		return nil, err
	}
	n := raft.New(id, peers)
	if recs, err := disk.LoadSynced(); err == nil && len(recs) > 0 {
		_ = n.RestoreHardState(recs[len(recs)-1])
	}
	if len(tokenSecret) == 0 {
		return nil, fmt.Errorf("token secret required")
	}
	return &Host{
		ID: id, Node: n, Disk: disk, MVCC: mvcc.New(), Sessions: session.Table{},
		KV: map[string]string{}, PeerAddrs: peerAddrs,
		Codec:   &token.Codec{Secret: tokenSecret, Epoch: 1, Group: 1},
		Snap:    &snapshot.Store{},
		waiting: map[string]chan struct{}{},
	}, nil
}

// TokenOK is constant-time equality for cluster auth tokens.
func TokenOK(got, want string) bool {
	return hmac.Equal([]byte(got), []byte(want))
}

func (h *Host) StartPeers(listen string) error {
	var ln net.Listener
	var err error
	if h.TLS != nil {
		ln, err = tls.Listen("tcp", listen, h.TLS)
	} else {
		ln, err = net.Listen("tcp", listen)
	}
	if err != nil {
		return err
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go h.readPeer(c)
		}
	}()
	go h.tickLoop()
	return nil
}

func (h *Host) tickLoop() {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		h.mu.Lock()
		h.applyEffects(h.Node.Step(runtime.Tick{Kind: "election"}))
		h.mu.Unlock()
	}
}

func (h *Host) readPeer(c net.Conn) {
	defer c.Close()
	dec := json.NewDecoder(c)
	if h.AuthToken != "" {
		var a struct {
			Auth string `json:"auth"`
		}
		if dec.Decode(&a) != nil || !TokenOK(a.Auth, h.AuthToken) {
			return
		}
	}
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return
		}
		h.mu.Lock()
		h.applyEffects(h.Node.Step(runtime.PeerMessage{Payload: raw}))
		h.drainPendingSnap()
		h.mu.Unlock()
	}
}

func (h *Host) applyEffects(effs []runtime.Effect) {
	for _, eff := range effs {
		switch e := eff.(type) {
		case runtime.Persist:
			if err := h.Disk.Append(e.Data); err != nil {
				panic(fmt.Errorf("wal append: %w", err))
			}
		case runtime.Sync:
			if h.Disk.Sync() != nil {
				continue
			}
			h.applyEffects(h.Node.Step(runtime.DiskCompletion{RequestID: e.RequestID, Synced: true}))
		case runtime.Send:
			go h.sendPeer(e.To, e.Payload)
		case runtime.Apply:
			h.onApply(e)
		case runtime.Reply:
			if ch := h.waiting[e.RequestID]; ch != nil {
				close(ch)
				delete(h.waiting, e.RequestID)
			}
		}
	}
}

func (h *Host) onApply(a runtime.Apply) {
	var cmd map[string]any
	if json.Unmarshal(a.Command, &cmd) != nil {
		return
	}
	kind, _ := cmd["kind"].(string)
	switch kind {
	case "Put":
		key, _ := cmd["key"].(string)
		val, _ := cmd["value"].(string)
		sess, _ := cmd["session"].(string)
		if sess != "" {
			seq := uint64(num(cmd["seq"]))
			dig, _ := cmd["digest"].(string)
			if cached, _, err := h.Sessions.Decide(sess, seq, dig); err != nil || cached != nil {
				h.MVCC.SetApplied(mvcc.Revision(a.Index))
				return
			}
			h.KV[key] = val
			h.Sessions.Put(sess, seq, dig, []byte("ok"), a.Index)
		} else {
			h.KV[key] = val
		}
		h.MVCC.SetApplied(mvcc.Revision(a.Index))
	case "Fence", "Noop":
		h.MVCC.SetApplied(mvcc.Revision(a.Index))
	case "TupleBatch":
		store, _ := cmd["store"].(string)
		ops, _ := cmd["ops"].([]any)
		for _, raw := range ops {
			op, _ := raw.(map[string]any)
			k := mvcc.EncodeKey(store, str(op["ons"]), str(op["oid"]), str(op["rel"]),
				str(op["sns"]), str(op["sid"]), str(op["srel"]))
			if del, _ := op["delete"].(bool); del {
				mvcc.MustApply(h.MVCC.ApplyDelete(mvcc.Revision(a.Index), k))
			} else {
				mvcc.MustApply(h.MVCC.ApplyPut(mvcc.Revision(a.Index), k))
			}
		}
		if sess, _ := cmd["session"].(string); sess != "" {
			seq := uint64(num(cmd["seq"]))
			dig, _ := cmd["digest"].(string)
			h.Sessions.Put(sess, seq, dig, []byte("ok"), a.Index)
		}
	}
	h.maybePublishSnapshot()
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func (h *Host) maybePublishSnapshot() {
	if h.Node.LastApplied() < 100 || h.Node.LastApplied()-h.Node.SnapIndex() < 100 {
		return
	}
	idx := h.Node.LastApplied()
	term := h.Node.Term()
	sess, _ := json.Marshal(h.Sessions)
	kv := make(map[string]string, len(h.KV))
	for k, v := range h.KV {
		kv[k] = v
	}
	_ = h.Snap.BeginWrite(snapshot.State{
		Meta:     snapshot.Meta{LastIndex: idx, LastTerm: term},
		KV:       kv,
		Sessions: sess,
	})
	h.Snap.Publish()
	h.Node.CompactLog(idx, term)
	h.Node.SetSnapData(h.Snap.Bytes())
}

func (h *Host) drainPendingSnap() {
	raw := h.Node.PendingSnap()
	if len(raw) == 0 {
		return
	}
	if err := h.Snap.Install(raw); err != nil {
		panic(err)
	}
	st, err := h.Snap.Load()
	if err != nil {
		return
	}
	h.KV = st.KV
	if h.KV == nil {
		h.KV = map[string]string{}
	}
	if len(st.Sessions) > 0 {
		_ = json.Unmarshal(st.Sessions, &h.Sessions)
	}
}

func (h *Host) sendPeer(to runtime.NodeID, payload []byte) {
	addr, ok := h.PeerAddrs[to]
	if !ok {
		return
	}
	var c net.Conn
	var err error
	if h.TLS != nil {
		d := &net.Dialer{Timeout: 200 * time.Millisecond}
		cfg := h.TLS.Clone()
		if cfg.ServerName == "" {
			cfg.InsecureSkipVerify = true // ponytail: same-cert cluster; add CA verify if a cluster CA exists
		}
		c, err = tls.DialWithDialer(d, "tcp", addr, cfg)
	} else {
		c, err = net.DialTimeout("tcp", addr, 200*time.Millisecond)
	}
	if err != nil {
		return
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(200 * time.Millisecond))
	enc := json.NewEncoder(c)
	if h.AuthToken != "" {
		_ = enc.Encode(struct {
			Auth string `json:"auth"`
		}{Auth: h.AuthToken})
	}
	_ = enc.Encode(json.RawMessage(payload))
}

// Propose waits until the client command is replied or timeout.
func (h *Host) Propose(reqID string, payload []byte, wait time.Duration) error {
	ch := make(chan struct{})
	h.mu.Lock()
	if h.Node.Role() != raft.Leader {
		h.mu.Unlock()
		return fmt.Errorf("not leader")
	}
	h.waiting[reqID] = ch
	h.applyEffects(h.Node.Step(runtime.ClientCommand{RequestID: reqID, Payload: payload}))
	h.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-time.After(wait):
		h.mu.Lock()
		delete(h.waiting, reqID)
		h.mu.Unlock()
		return fmt.Errorf("timeout")
	}
}

func (h *Host) Check(store, ons, oid, rel, sns, sid string) (bool, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if idx, ok := h.Node.ReadIndex(); ok {
		h.MVCC.SetApplied(mvcc.Revision(idx))
	}
	if h.MVCC.Applied() < mvcc.Revision(h.Node.LastApplied()) {
		h.MVCC.SetApplied(mvcc.Revision(h.Node.LastApplied()))
	}
	rev := h.MVCC.Applied()
	if rev == 0 {
		return false, "", fmt.Errorf("no revision")
	}
	snap, err := h.MVCC.Open(rev)
	if err != nil {
		return false, "", err
	}
	defer snap.Close()
	eng := &authz.Engine{Snap: snap}
	res, err := eng.Check(store, ons, oid, rel, sns, sid)
	if err != nil || res == authz.Incomplete {
		return false, "", fmt.Errorf("incomplete: %v", err)
	}
	return res == authz.Allow, h.Codec.Mint(store, uint64(rev)), nil
}

func (h *Host) ListSubjects(store, ons, oid, rel string, limit int) ([]authz.Tuple, uint64, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rev := h.MVCC.Applied()
	if rev == 0 {
		return nil, 0, "", fmt.Errorf("no revision")
	}
	snap, err := h.MVCC.Open(rev)
	if err != nil {
		return nil, 0, "", err
	}
	defer snap.Close()
	eng := &authz.Engine{Snap: snap}
	subs, err := eng.ListSubjects(store, ons, oid, rel, limit)
	if err != nil {
		return nil, uint64(rev), "", err
	}
	return subs, uint64(rev), h.Codec.Mint(store, uint64(rev)), nil
}

func (h *Host) Status() (role string, term, commit, applied uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch h.Node.Role() {
	case raft.Leader:
		role = "leader"
	case raft.Candidate:
		role = "candidate"
	default:
		role = "follower"
	}
	return role, h.Node.Term(), h.Node.CommitIndex(), h.Node.LastApplied()
}

func (h *Host) MuLock()    { h.mu.Lock() }
func (h *Host) MuUnlock()  { h.mu.Unlock() }
func (h *Host) MuRLock()   { h.mu.RLock() }
func (h *Host) MuRUnlock() { h.mu.RUnlock() }

// ApplyEffects applies runtime effects (exported for admin RPCs).
func (h *Host) ApplyEffects(effs []runtime.Effect) { h.applyEffects(effs) }
