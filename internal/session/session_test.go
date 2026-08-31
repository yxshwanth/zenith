package session

import "testing"

func TestDecideIdempotent(t *testing.T) {
	tab := Table{}
	tab.Put("c1", 1, "d", []byte("ok"), 5)
	got, rev, err := tab.Decide("c1", 1, "d")
	if err != nil || string(got) != "ok" || rev != 5 {
		t.Fatalf("got %q rev=%d err=%v", got, rev, err)
	}
}

func TestDecideDigestMismatch(t *testing.T) {
	tab := Table{}
	tab.Put("c1", 1, "d", []byte("ok"), 5)
	_, _, err := tab.Decide("c1", 1, "other")
	if err == nil {
		t.Fatal("expected digest mismatch error")
	}
}
