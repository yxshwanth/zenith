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
