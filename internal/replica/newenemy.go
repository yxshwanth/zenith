package replica

import (
	"fmt"

	v2 "github.com/yxshwanth/zenith/api/zenith/v2"
	contentservice "github.com/yxshwanth/zenith/examples/content-service"
	"github.com/yxshwanth/zenith/internal/checker"
	"github.com/yxshwanth/zenith/internal/mvcc"
	"github.com/yxshwanth/zenith/internal/runtime"
)

// RunNewEnemy is the A3 adversary: revoke, fence, publish, then reject a stale check.
func RunNewEnemy(seed uint64) error {
	c := NewCluster(seed, []runtime.NodeID{1, 2, 3})
	leader := c.BootstrapElect(50)
	if leader == 0 {
		return fmt.Errorf("no leader")
	}
	c.ProposeTuple(leader, "g1", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob",
	})
	c.Run(800)
	c.ProposeTuple(leader, "g2", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "writer",
		SubjectNamespace: "user", SubjectID: "charlie",
	})
	c.Run(800)
	preRevoke := c.Replicas[leader].MVCC.Applied()

	c.ProposeTuple(leader, "rev", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob", Delete: true,
	})
	c.Run(1500)
	revR := uint64(c.Replicas[leader].MVCC.Applied())
	if mvcc.Revision(revR) <= preRevoke {
		return fmt.Errorf("revoke did not advance applied: before=%d after=%d", preRevoke, revR)
	}

	ccc := c.CheckContentChange(leader, v2.CheckContentChangeRequest{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "writer",
		SubjectNamespace: "user", SubjectID: "charlie", RequestID: "ccc1",
	})
	if ccc.Err != nil || !ccc.Allowed || ccc.Token == "" {
		return fmt.Errorf("CheckContentChange: %+v", ccc)
	}
	revF := ccc.Rev
	if revF < revR {
		return fmt.Errorf("fence rev %d < revoke %d", revF, revR)
	}

	store := contentservice.New()
	if err := store.Publish("v2", "doc:1", []byte("new"), ccc.Token); err != nil {
		return err
	}

	deny := c.CheckPermission(leader, v2.CheckRequest{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob",
		Consistency: v2.Consistency{Mode: v2.FullyConsistent},
	})
	if deny.Allowed {
		return fmt.Errorf("bob must be DENY after revoke")
	}

	stale := c.CheckPermission(leader, v2.CheckRequest{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "bob",
		Consistency:   v2.Consistency{Mode: v2.AtExactRevision, Revision: uint64(preRevoke)},
		RequiredToken: ccc.Token,
	})
	if stale.Allowed {
		return fmt.Errorf("must not ALLOW at pre-revoke revision when required token is fence")
	}
	if stale.Err == nil {
		return fmt.Errorf("expected error: cannot satisfy token with older exact revision")
	}

	var lag runtime.NodeID
	for id := range c.Replicas {
		if id != leader {
			lag = id
			break
		}
	}
	lagApplied := c.Replicas[lag].MVCC.Applied()
	c.Frozen[lag] = true
	c.ProposeTuple(leader, "bump", command{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
		SubjectNamespace: "user", SubjectID: "alice",
	})
	c.Run(1500)
	ccc2 := c.CheckContentChange(leader, v2.CheckContentChangeRequest{
		Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "writer",
		SubjectNamespace: "user", SubjectID: "charlie", RequestID: "ccc2",
	})
	if ccc2.Token == "" || c.Replicas[lag].MVCC.Applied() != lagApplied {
		if c.Replicas[lag].MVCC.Applied() >= mvcc.Revision(ccc2.Rev) && ccc2.Rev > uint64(lagApplied) {
			return fmt.Errorf("frozen lag advanced: %d -> %d", lagApplied, c.Replicas[lag].MVCC.Applied())
		}
	}
	if ccc2.Token != "" {
		behind := c.CheckPermission(lag, v2.CheckRequest{
			Store: "s", ObjectNamespace: "doc", ObjectID: "1", Relation: "viewer",
			SubjectNamespace: "user", SubjectID: "bob",
			Consistency:   v2.Consistency{Mode: v2.AtLeastAsFresh},
			RequiredToken: ccc2.Token,
		})
		if behind.Allowed || behind.Err == nil {
			return fmt.Errorf("frozen lag must error behind, got allowed=%v err=%v", behind.Allowed, behind.Err)
		}
	}

	events := []checker.ContentEvent{
		{Kind: checker.ContentRevoke, Rev: revR},
		{Kind: checker.ContentFence, Rev: revF},
		{Kind: checker.ContentPublish, Rev: revF, Version: "v2"},
		{Kind: checker.ContentCheck, EvalRev: uint64(preRevoke), Allowed: false},
	}
	if err := checker.NewEnemyOK(events); err != nil {
		return err
	}

	codec := c.TokenCodec()
	_, err := contentservice.Read(store, codec, "s", "v2", "bob", func(req uint64, reader, object string) (bool, uint64, error) {
		return true, req - 1, nil
	})
	if err == nil {
		return fmt.Errorf("content serve must reject stale eval")
	}
	return nil
}
