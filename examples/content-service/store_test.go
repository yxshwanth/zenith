package contentservice

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/token"
)

func TestPublishRequiresToken(t *testing.T) {
	s := New()
	if err := s.Publish("v1", "doc:1", []byte("x"), ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestReadEnforcesTokenFreshness(t *testing.T) {
	codec := &token.Codec{Secret: []byte("k"), Epoch: 1, Group: 1}
	s := New()
	tok := codec.Mint("s", 42)
	_ = s.Publish("v2", "doc:1", []byte("secret"), tok)

	_, err := Read(s, codec, "s", "v2", "bob", func(req uint64, reader, object string) (bool, uint64, error) {
		return true, 40, nil // stale eval
	})
	if err == nil {
		t.Fatal("stale eval must not serve")
	}

	got, err := Read(s, codec, "s", "v2", "bob", func(req uint64, reader, object string) (bool, uint64, error) {
		return false, 42, nil
	})
	if err == nil || got != nil {
		t.Fatal("DENY must not serve")
	}

	got, err = Read(s, codec, "s", "v2", "bob", func(req uint64, reader, object string) (bool, uint64, error) {
		return true, 42, nil
	})
	if err != nil || string(got) != "secret" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
