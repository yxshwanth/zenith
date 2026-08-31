package checker

import "testing"

func TestNewEnemyOK(t *testing.T) {
	ok := []ContentEvent{
		{Kind: ContentRevoke, Rev: 41},
		{Kind: ContentFence, Rev: 42},
		{Kind: ContentPublish, Rev: 42, Version: "v2"},
		{Kind: ContentCheck, EvalRev: 42, Allowed: false},
	}
	if err := NewEnemyOK(ok); err != nil {
		t.Fatal(err)
	}
	bad := []ContentEvent{
		{Kind: ContentRevoke, Rev: 41},
		{Kind: ContentFence, Rev: 42},
		{Kind: ContentPublish, Rev: 42, Version: "v2"},
		{Kind: ContentCheck, EvalRev: 40, Allowed: true},
	}
	if err := NewEnemyOK(bad); err == nil {
		t.Fatal("expected violation")
	}
}
