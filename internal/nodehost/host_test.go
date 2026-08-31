package nodehost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yxshwanth/zenith/internal/runtime"
	"github.com/yxshwanth/zenith/internal/snapshot"
)

func testHost(t *testing.T) *Host {
	t.Helper()
	h, err := Open(1, []runtime.NodeID{1}, nil, t.TempDir(), []byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestOpenStatus(t *testing.T) {
	h := testHost(t)
	role, term, _, _ := h.Status()
	if role != "follower" || term != 0 {
		t.Fatalf("role=%s term=%d", role, term)
	}
}

func TestTokenOK(t *testing.T) {
	if !TokenOK("abc", "abc") {
		t.Fatal("equal tokens")
	}
	if TokenOK("abc", "abd") {
		t.Fatal("unequal tokens")
	}
	if TokenOK("", "x") {
		t.Fatal("empty vs set")
	}
}

func TestWALAppendFailurePanics(t *testing.T) {
	dir := t.TempDir()
	h, err := Open(1, []runtime.NodeID{1}, nil, dir, []byte("s"))
	if err != nil {
		t.Fatal(err)
	}
	wal := filepath.Join(dir, "wal.log")
	if err := os.Remove(wal); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(wal, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("want panic on wal append")
		}
	}()
	h.ApplyEffects([]runtime.Effect{runtime.Persist{Data: []byte("x")}})
}

func TestMVCCApplyOrderPanics(t *testing.T) {
	h := testHost(t)
	batch, _ := json.Marshal(map[string]any{
		"kind": "TupleBatch", "store": "s",
		"ops": []any{map[string]any{"ons": "doc", "oid": "1", "rel": "viewer", "sns": "user", "sid": "bob"}},
	})
	h.ApplyEffects([]runtime.Effect{runtime.Apply{Index: 2, Command: batch}})
	defer func() {
		if recover() == nil {
			t.Fatal("want panic on rev < applied")
		}
	}()
	h.ApplyEffects([]runtime.Effect{runtime.Apply{Index: 1, Command: batch}})
}

func TestInstallSnapshotRestoresKV(t *testing.T) {
	h := testHost(t)
	st := snapshot.State{Meta: snapshot.Meta{LastIndex: 3, LastTerm: 1}, KV: map[string]string{"k": "v"}}
	if err := h.Snap.BeginWrite(st); err != nil {
		t.Fatal(err)
	}
	h.Snap.Publish()
	payload, err := json.Marshal(map[string]any{
		"type": "InstallSnapshot", "term": 1, "from": 2,
		"snapIndex": 3, "snapTerm": 1, "snapData": h.Snap.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	h.applyEffects(h.Node.Step(runtime.PeerMessage{Payload: payload}))
	h.drainPendingSnap()
	h.mu.Unlock()
	if h.KV["k"] != "v" {
		t.Fatalf("kv=%v", h.KV)
	}
}
