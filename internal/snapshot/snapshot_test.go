package snapshot

import "testing"

func TestPublishAtomic(t *testing.T) {
	s := &Store{}
	st := State{Meta: Meta{LastIndex: 3, LastTerm: 1}, KV: map[string]string{"a": "1"}}
	if err := s.BeginWrite(st); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("should not load before publish")
	}
	s.CrashDropStaging()
	if _, err := s.Load(); err == nil {
		t.Fatal("crash before publish must leave no snapshot")
	}
	_ = s.BeginWrite(st)
	s.Publish()
	got, err := s.Load()
	if err != nil || got.KV["a"] != "1" || got.Meta.LastIndex != 3 {
		t.Fatalf("got %+v err=%v", got, err)
	}
	// new generation
	_ = s.BeginWrite(State{Meta: Meta{LastIndex: 5, LastTerm: 2}, KV: map[string]string{"a": "2"}})
	s.CrashDropStaging()
	got, _ = s.Load()
	if got.KV["a"] != "1" {
		t.Fatalf("crash during new write must keep old gen, got %v", got.KV)
	}
}
