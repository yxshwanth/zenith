package replica

import (
	"testing"

	v2 "github.com/yxshwanth/zenith/api/zenith/v2"
	"github.com/yxshwanth/zenith/internal/runtime"
)

func TestWriteTuplesAndListSubjects(t *testing.T) {
	c := NewCluster(70, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		t.Fatal("no leader")
	}
	w := c.WriteTuples(leader, v2.WriteTuplesRequest{
		Store: "s", Session: "sess1", Seq: 1, RequestID: "w1",
		Tuples: []v2.Tuple{
			{ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer", SubjectNamespace: "user", SubjectID: "a"},
			{ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer", SubjectNamespace: "user", SubjectID: "b"},
		},
	})
	if w.Err != nil || w.Rev == 0 || w.Token == "" {
		t.Fatalf("WriteTuples: %+v\n%s", w, c)
	}
	ls := c.ListSubjects(leader, v2.ListSubjectsRequest{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		Consistency: v2.Consistency{Mode: v2.FullyConsistent},
	})
	if ls.Err != nil || !ls.Complete || len(ls.Subjects) != 2 {
		t.Fatalf("ListSubjects: %+v", ls)
	}
	out := c.GetRequestOutcome(leader, v2.GetRequestOutcomeRequest{Session: "sess1", Seq: 1, RequestID: "o1"})
	if !out.Found || out.Rev == 0 {
		t.Fatalf("GetRequestOutcome: %+v", out)
	}
}
