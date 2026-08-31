package wal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileWALCrashRestart(t *testing.T) {
	dir := t.TempDir()
	f, err := OpenFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Append([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := f.Append([]byte("b")); err != nil { // unsynced
		t.Fatal(err)
	}
	// simulate crash: reopen and only load synced prefix by truncating file to synced
	_ = os.Truncate(filepath.Join(dir, "wal.log"), f.synced)
	f2, err := OpenFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := f2.LoadSynced()
	if err != nil || len(recs) != 1 || string(recs[0]) != "a" {
		t.Fatalf("got %#v err=%v", recs, err)
	}
}
