package checker

import "testing"

func TestKVModelGetMiss(t *testing.T) {
	m := NewKVModel()
	got := m.Apply(KVOp{Kind: KVGet, Key: "x"})
	want := KVResult{Found: false, Value: ""}
	if got != want {
		t.Fatalf("Get on empty model = %+v, want %+v", got, want)
	}
}

func TestKVModelPutThenGet(t *testing.T) {
	m := NewKVModel()
	m.Apply(KVOp{Kind: KVPut, Key: "x", Value: "1"})
	got := m.Apply(KVOp{Kind: KVGet, Key: "x"})
	want := KVResult{Found: true, Value: "1"}
	if got != want {
		t.Fatalf("Get after Put = %+v, want %+v", got, want)
	}
}

func TestKVModelDelete(t *testing.T) {
	m := NewKVModel()
	m.Apply(KVOp{Kind: KVPut, Key: "x", Value: "1"})
	del := m.Apply(KVOp{Kind: KVDelete, Key: "x"})
	if !del.Found {
		t.Fatalf("Delete of present key reported Found=false")
	}
	got := m.Apply(KVOp{Kind: KVGet, Key: "x"})
	if got.Found {
		t.Fatalf("Get after Delete = %+v, want Found=false", got)
	}
}

func TestKVModelCompareAndSwap(t *testing.T) {
	m := NewKVModel()
	m.Apply(KVOp{Kind: KVPut, Key: "x", Value: "1"})

	fail := m.Apply(KVOp{Kind: KVCompareAndSwap, Key: "x", Expect: "wrong", Swap: "2"})
	if fail.Swapped {
		t.Fatalf("CAS with wrong Expect reported Swapped=true")
	}
	if got := m.Apply(KVOp{Kind: KVGet, Key: "x"}); got.Value != "1" {
		t.Fatalf("value changed after a failed CAS: %+v", got)
	}

	ok := m.Apply(KVOp{Kind: KVCompareAndSwap, Key: "x", Expect: "1", Swap: "2"})
	if !ok.Swapped || ok.Value != "2" {
		t.Fatalf("CAS with correct Expect = %+v, want Swapped=true Value=2", ok)
	}
}
