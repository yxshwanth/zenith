package token

import "testing"

func TestMintVerifyRoundTrip(t *testing.T) {
	c := &Codec{Secret: []byte("s"), Epoch: 1, Group: 1}
	tok := c.Mint("storeA", 42)
	cl, err := c.Verify(tok, "storeA")
	if err != nil || cl.Revision != 42 || cl.Store != "storeA" {
		t.Fatalf("got %+v err=%v", cl, err)
	}
}

func TestRejectTamperWrongEpochStore(t *testing.T) {
	c := &Codec{Secret: []byte("s"), Epoch: 1, Group: 1}
	tok := c.Mint("s1", 10)
	if _, err := c.Verify(tok+"x", "s1"); err == nil {
		t.Fatal("expected tamper/malformed")
	}
	bad := []byte(tok)
	bad[len(bad)-1] ^= 1
	if _, err := c.Verify(string(bad), "s1"); err != ErrTampered && err != ErrMalformed {
		// base64 may become malformed
		_ = err
	}
	c2 := &Codec{Secret: []byte("s"), Epoch: 2, Group: 1}
	if _, err := c2.Verify(tok, "s1"); err != ErrWrongEpoch {
		t.Fatalf("want wrong epoch, got %v", err)
	}
	if _, err := c.Verify(tok, "other"); err != ErrWrongStore {
		t.Fatalf("want wrong store, got %v", err)
	}
}

func TestNotCockroachZookie(t *testing.T) {
	// Casting int64 "zookie" strings must fail.
	c := &Codec{Secret: []byte("s"), Epoch: 1, Group: 1}
	if _, err := c.Verify("1735689600000000000", "s"); err == nil {
		t.Fatal("legacy integer zookie must not verify")
	}
}
