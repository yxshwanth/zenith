package wal

import "testing"

func TestEncodeRecoverRoundTrip(t *testing.T) {
	var buf []byte
	buf = Encode(buf, []byte("hello"))
	buf = Encode(buf, []byte("world"))
	recs, err := Recover(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || string(recs[0]) != "hello" || string(recs[1]) != "world" {
		t.Fatalf("got %#v", recs)
	}
}

func TestRecoverTornTail(t *testing.T) {
	var buf []byte
	buf = Encode(buf, []byte("ok"))
	buf = append(buf, 0x5a, 0x4e) // torn header
	recs, err := Recover(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || string(recs[0]) != "ok" {
		t.Fatalf("want one record, got %#v", recs)
	}
}

func TestMemoryCrashUnsynced(t *testing.T) {
	m := &Memory{}
	m.Append([]byte("a"))
	m.Sync()
	m.Append([]byte("b")) // not synced
	m.CrashUnsynced()
	recs, err := m.LoadSynced()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || string(recs[0]) != "a" {
		t.Fatalf("got %#v", recs)
	}
}
