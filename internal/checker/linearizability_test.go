package checker

import "testing"

// TestLinearizableKVAcceptsLegalHistory is the "known legal" fixture:
// two non-overlapping operations whose
// recorded results are exactly what sequential application produces.
func TestLinearizableKVAcceptsLegalHistory(t *testing.T) {
	history := []HistoryEntry{
		{
			Op:          KVOp{Kind: KVPut, Key: "x", Value: "1"},
			Result:      KVResult{Found: true, Value: "1"},
			InvokedAt:   0,
			CompletedAt: 10,
		},
		{
			Op:          KVOp{Kind: KVGet, Key: "x"},
			Result:      KVResult{Found: true, Value: "1"},
			InvokedAt:   20,
			CompletedAt: 30,
		},
	}

	ok, witness := LinearizableKV(history)
	if !ok {
		t.Fatalf("expected a legal, non-overlapping history to be accepted")
	}
	if len(witness) != 2 || witness[0] != 0 || witness[1] != 1 {
		t.Fatalf("expected witness order [0 1], got %v", witness)
	}
}

// TestLinearizableKVAcceptsConcurrentLegalHistory checks that two
// overlapping, mutually consistent operations (either order reproduces
// both results) are accepted.
func TestLinearizableKVAcceptsConcurrentLegalHistory(t *testing.T) {
	history := []HistoryEntry{
		{
			Op:          KVOp{Kind: KVPut, Key: "x", Value: "1"},
			Result:      KVResult{Found: true, Value: "1"},
			InvokedAt:   0,
			CompletedAt: 100, // overlaps with the Get below
		},
		{
			Op:          KVOp{Kind: KVGet, Key: "y"}, // unrelated key: any order works
			Result:      KVResult{Found: false},
			InvokedAt:   10,
			CompletedAt: 20,
		},
	}

	ok, _ := LinearizableKV(history)
	if !ok {
		t.Fatalf("expected an overlapping but mutually consistent history to be accepted")
	}
}

// TestLinearizableKVRejectsIllegalHistory is the "known illegal" fixture:
// a Get reports a value that was never written by any operation in the
// history, under any legal order. This proves the checker can actually
// detect a violation rather than accepting everything.
func TestLinearizableKVRejectsIllegalHistory(t *testing.T) {
	history := []HistoryEntry{
		{
			Op:          KVOp{Kind: KVPut, Key: "x", Value: "1"},
			Result:      KVResult{Found: true, Value: "1"},
			InvokedAt:   0,
			CompletedAt: 10,
		},
		{
			Op:          KVOp{Kind: KVGet, Key: "x"},
			Result:      KVResult{Found: true, Value: "2"}, // never written
			InvokedAt:   20,
			CompletedAt: 30,
		},
	}

	ok, witness := LinearizableKV(history)
	if ok {
		t.Fatalf("expected an impossible history to be rejected, got witness order %v", witness)
	}
}

// TestLinearizableKVRejectsStaleReadAfterCompletedWrite checks the
// real-time ordering constraint itself: a Get that starts after a Put
// completes must observe that Put, even though a value-only replay could
// otherwise "explain" the stale result by reordering.
func TestLinearizableKVRejectsStaleReadAfterCompletedWrite(t *testing.T) {
	history := []HistoryEntry{
		{
			Op:          KVOp{Kind: KVPut, Key: "x", Value: "1"},
			Result:      KVResult{Found: true, Value: "1"},
			InvokedAt:   0,
			CompletedAt: 10,
		},
		{
			Op:          KVOp{Kind: KVPut, Key: "x", Value: "2"},
			Result:      KVResult{Found: true, Value: "2"},
			InvokedAt:   20,
			CompletedAt: 30,
		},
		{
			// Invoked after the second Put completed, so real-time order
			// requires it to observe "2" — but it claims "1".
			Op:          KVOp{Kind: KVGet, Key: "x"},
			Result:      KVResult{Found: true, Value: "1"},
			InvokedAt:   40,
			CompletedAt: 50,
		},
	}

	ok, witness := LinearizableKV(history)
	if ok {
		t.Fatalf("expected a real-time-violating history to be rejected, got witness order %v", witness)
	}
}

func TestCheckKVHistoryPassFailInconclusive(t *testing.T) {
	legal := []HistoryEntry{
		{Op: KVOp{Kind: KVPut, Key: "x", Value: "1"}, Result: KVResult{Found: true, Value: "1"}, InvokedAt: 0, CompletedAt: 10},
		{Op: KVOp{Kind: KVGet, Key: "x"}, Result: KVResult{Found: true, Value: "1"}, InvokedAt: 20, CompletedAt: 30},
	}
	if got := CheckKVHistory(legal); got != Pass {
		t.Fatalf("legal history: want pass, got %s", got)
	}

	illegal := []HistoryEntry{
		{Op: KVOp{Kind: KVPut, Key: "x", Value: "1"}, Result: KVResult{Found: true, Value: "1"}, InvokedAt: 0, CompletedAt: 10},
		{Op: KVOp{Kind: KVGet, Key: "x"}, Result: KVResult{Found: true, Value: "2"}, InvokedAt: 20, CompletedAt: 30},
	}
	if got := CheckKVHistory(illegal); got != Fail {
		t.Fatalf("illegal history: want fail, got %s", got)
	}

	// Unknown op alone → inconclusive (never pass).
	unknownOnly := []HistoryEntry{
		{Op: KVOp{Kind: KVPut, Key: "x", Value: "1"}, InvokedAt: 0, CompletedAt: 10, Unknown: true},
	}
	if got := CheckKVHistory(unknownOnly); got != Inconclusive {
		t.Fatalf("unknown-only: want inconclusive, got %s", got)
	}

	// Unknown + illegal resolved subset → fail (do not hide a bug behind timeout).
	unknownPlusIllegal := []HistoryEntry{
		{Op: KVOp{Kind: KVPut, Key: "x", Value: "1"}, Result: KVResult{Found: true, Value: "1"}, InvokedAt: 0, CompletedAt: 10},
		{Op: KVOp{Kind: KVGet, Key: "x"}, Result: KVResult{Found: true, Value: "9"}, InvokedAt: 20, CompletedAt: 30},
		{Op: KVOp{Kind: KVPut, Key: "y", Value: "1"}, InvokedAt: 40, CompletedAt: 50, Unknown: true},
	}
	if got := CheckKVHistory(unknownPlusIllegal); got != Fail {
		t.Fatalf("unknown+illegal: want fail, got %s", got)
	}

	// Unknown + legal resolved subset → inconclusive.
	unknownPlusLegal := []HistoryEntry{
		{Op: KVOp{Kind: KVPut, Key: "x", Value: "1"}, Result: KVResult{Found: true, Value: "1"}, InvokedAt: 0, CompletedAt: 10},
		{Op: KVOp{Kind: KVPut, Key: "y", Value: "1"}, InvokedAt: 20, CompletedAt: 30, Unknown: true},
	}
	if got := CheckKVHistory(unknownPlusLegal); got != Inconclusive {
		t.Fatalf("unknown+legal: want inconclusive, got %s", got)
	}

	// Unacked Put of k=v1 then completed Get of k=v1 is legal (Inconclusive).
	inflightExplainsGet := []HistoryEntry{
		{Op: KVOp{Kind: KVPut, Key: "k0", Value: "v1"}, InvokedAt: 33, Unknown: true},
		{Op: KVOp{Kind: KVGet, Key: "k0"}, Result: KVResult{Found: true, Value: "v1"}, InvokedAt: 40, CompletedAt: 41},
	}
	if got := CheckKVHistory(inflightExplainsGet); got != Inconclusive {
		t.Fatalf("inflight put explains get: want inconclusive, got %s", got)
	}
}
