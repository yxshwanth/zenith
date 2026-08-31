package coordinator

import (
	"testing"

	v2 "github.com/yxshwanth/zenith/api/zenith/v2"
	"github.com/yxshwanth/zenith/internal/mvcc"
	"github.com/yxshwanth/zenith/internal/token"
)

func TestFullyConsistentUsesApplied(t *testing.T) {
	r, err := SelectRevision(v2.Consistency{Mode: v2.FullyConsistent}, State{Applied: 10, Floor: 0}, nil)
	if err != nil || r != 10 {
		t.Fatalf("got %d %v", r, err)
	}
}

func TestAtLeastAsFreshBehindErrors(t *testing.T) {
	_, err := SelectRevision(v2.Consistency{Mode: v2.AtLeastAsFresh, Revision: 20}, State{Applied: 10, Floor: 0}, nil)
	if err == nil {
		t.Fatal("expected behind error")
	}
}

func TestAtLeastAsFreshUsesTokenMin(t *testing.T) {
	r, err := SelectRevision(
		v2.Consistency{Mode: v2.AtLeastAsFresh, Revision: 5},
		State{Applied: 15, Floor: 0},
		&token.Claims{Revision: 12},
	)
	if err != nil || r != 15 {
		t.Fatalf("got %d %v", r, err)
	}
}

func TestAtExactRevisionCompacted(t *testing.T) {
	_, err := SelectRevision(v2.Consistency{Mode: v2.AtExactRevision, Revision: 2}, State{Applied: 10, Floor: 5}, nil)
	if err == nil {
		t.Fatal("expected compacted")
	}
}

func TestAtExactRevisionHit(t *testing.T) {
	r, err := SelectRevision(v2.Consistency{Mode: v2.AtExactRevision, Revision: 7}, State{Applied: 10, Floor: 0}, nil)
	if err != nil || r != 7 {
		t.Fatalf("got %d %v", r, err)
	}
	_ = mvcc.Revision(0)
}
