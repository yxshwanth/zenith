package checker

import "testing"

func TestPrefixLedgerRejectsConflict(t *testing.T) {
	p := PrefixLedger{}
	if !p.Observe(1, 1, "a") {
		t.Fatal("first observe")
	}
	if p.Observe(1, 1, "b") {
		t.Fatal("conflicting hash must fail")
	}
}

func TestPrefixLedgerAllowsDifferentTermsAtSameIndex(t *testing.T) {
	p := PrefixLedger{}
	if !p.Observe(5, 1, "old") {
		t.Fatal("term 1")
	}
	if !p.Observe(5, 2, "new") {
		t.Fatal("term 2 overwrite is not an R3 conflict")
	}
	if !p.Observe(5, 1, "old") {
		t.Fatal("same (index,term) must still match")
	}
}
