package deccache_test

import (
	"testing"

	"github.com/yxshwanth/zenith/internal/deccache"
)

func TestDecisionCacheRespectsRevision(t *testing.T) {
	c := deccache.New()
	k1 := deccache.Key{Store: "s", ONS: "doc", OID: "1", Rel: "viewer", SNS: "user", SID: "a", Revision: 5}
	c.Put(k1, true)
	if e, ok := c.Get(k1); !ok || !e.Allowed {
		t.Fatal("expected hit at rev 5")
	}
	kStale := k1
	kStale.Revision = 4
	if _, ok := c.Get(kStale); ok {
		t.Fatal("must not hit different revision (C1)")
	}
	k2 := k1
	k2.Revision = 6
	if _, ok := c.Get(k2); ok {
		t.Fatal("must not hit newer revision without put")
	}
}
