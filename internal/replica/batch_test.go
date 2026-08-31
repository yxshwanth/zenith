package replica

import (
	"testing"

	v2 "github.com/yxshwanth/zenith/api/zenith/v2"
	"github.com/yxshwanth/zenith/internal/runtime"
)

// TestBatchCheckSameRevision is A4: all batch items share one snapshot revision.
func TestBatchCheckSameRevision(t *testing.T) {
	c := NewCluster(55, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	c.ProposeTuple(leader, "b1", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "alice",
	})
	c.ProposeTuple(leader, "b2", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "2", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob",
	})
	c.Run(1500)

	batch := c.BatchCheck(leader, v2.BatchCheckRequest{
		Store:       "s",
		Consistency: v2.Consistency{Mode: v2.FullyConsistent},
		Items: []v2.CheckRequest{
			{ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer", SubjectNamespace: "user", SubjectID: "alice",
				Consistency: v2.Consistency{Mode: v2.AtExactRevision, Revision: 1}}, // ignored per-item
			{ObjectNamespace: "doc", ObjectID: "2", Relation: "viewer", SubjectNamespace: "user", SubjectID: "bob"},
			{ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer", SubjectNamespace: "user", SubjectID: "bob"},
		},
	})
	if batch.Err != nil {
		t.Fatalf("BatchCheck: %v\n%s", batch.Err, c)
	}
	if batch.Revision == 0 || len(batch.Results) != 3 {
		t.Fatalf("bad response: %+v", batch)
	}
	if !batch.Results[0].Allowed || !batch.Results[1].Allowed || batch.Results[2].Allowed {
		t.Fatalf("want allow,allow,deny got %+v", batch.Results)
	}
	for i, r := range batch.Results {
		if r.Token != batch.Token {
			t.Fatalf("item %d token %q != batch %q (A4)", i, r.Token, batch.Token)
		}
	}
	// Second batch at same applied must report identical revision when using exact.
	exact := c.BatchCheck(leader, v2.BatchCheckRequest{
		Store:       "s",
		Consistency: v2.Consistency{Mode: v2.AtExactRevision, Revision: batch.Revision},
		Items:       []v2.CheckRequest{{ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer", SubjectNamespace: "user", SubjectID: "alice"}},
	})
	if exact.Err != nil || exact.Revision != batch.Revision {
		t.Fatalf("exact batch rev=%d want %d err=%v", exact.Revision, batch.Revision, exact.Err)
	}
}
