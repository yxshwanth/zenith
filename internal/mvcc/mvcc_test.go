package mvcc

import "testing"

func TestVisibilityAndTombstone(t *testing.T) {
	db := New()
	k := EncodeKey("s", "doc", "1", "viewer", "user", "bob", "")
	_ = db.ApplyPut(1, k)
	_ = db.ApplyPut(2, EncodeKey("s", "doc", "1", "viewer", "user", "alice", ""))
	_ = db.ApplyDelete(3, k)

	s1, err := db.Open(1)
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := s1.Contains(k)
	if !ok {
		t.Fatal("bob should be visible at r=1")
	}
	s1.Close()

	s3, _ := db.Open(3)
	ok, _ = s3.Contains(k)
	if ok {
		t.Fatal("bob tombstoned at r=3")
	}
	s3.Close()
}

func TestGCRefusesPinned(t *testing.T) {
	db := New()
	k := EncodeKey("s", "doc", "1", "viewer", "user", "bob", "")
	_ = db.ApplyPut(1, k)
	_ = db.ApplyPut(5, k)
	s, _ := db.Open(1)
	if err := db.GC(3); err == nil {
		t.Fatal("expected GC refuse while pin at 1")
	}
	s.Close()
	if err := db.GC(3); err != nil {
		t.Fatal(err)
	}
}

func TestMixedRevisionRejected(t *testing.T) {
	db := New()
	_ = db.ApplyPut(1, EncodeKey("s", "a", "1", "r", "u", "x", ""))
	s, _ := db.Open(1)
	s.Close()
	_, err := s.Contains(EncodeKey("s", "a", "1", "r", "u", "x", ""))
	if err == nil {
		t.Fatal("closed snapshot must error")
	}
}
