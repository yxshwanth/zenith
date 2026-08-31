package checker

import "testing"

func TestPorcupineAgreesOnLegalIllegal(t *testing.T) {
	legal := []HistoryEntry{
		{Op: KVOp{Kind: KVPut, Key: "x", Value: "1"}, Result: KVResult{Found: true, Value: "1"}, InvokedAt: 0, CompletedAt: 10},
		{Op: KVOp{Kind: KVGet, Key: "x"}, Result: KVResult{Found: true, Value: "1"}, InvokedAt: 20, CompletedAt: 30},
	}
	if CheckKVPorcupine(legal) != Pass {
		t.Fatal("legal should pass")
	}
	illegal := []HistoryEntry{
		{Op: KVOp{Kind: KVPut, Key: "x", Value: "1"}, Result: KVResult{Found: true, Value: "1"}, InvokedAt: 0, CompletedAt: 10},
		{Op: KVOp{Kind: KVGet, Key: "x"}, Result: KVResult{Found: true, Value: "9"}, InvokedAt: 20, CompletedAt: 30},
	}
	if CheckKVPorcupine(illegal) != Fail {
		t.Fatal("illegal should fail")
	}
}
