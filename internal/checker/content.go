package checker

import "fmt"

// ContentEvent is one step in the content-change protocol oracle (A3).
type ContentEventKind int

const (
	ContentRevoke ContentEventKind = iota
	ContentFence
	ContentPublish
	ContentCheck
)

// ContentEvent records causal steps for new-enemy validation.
type ContentEvent struct {
	Kind    ContentEventKind
	Rev     uint64 // for Revoke/Fence: committed revision; for Publish: required token rev
	Reader  string
	Allowed bool // Check outcome
	EvalRev uint64
	Version string // content version id
}

// NewEnemyOK reports whether history satisfies:
//
//	rev(R) < rev(F) = required_revision(C)
//	evaluation_revision(Check(C)) >= required_revision(C)
//
// and that a Check of C never ALLOWs with EvalRev < required(C) when the
// only path was removed by R (caller supplies that graph fact separately).
func NewEnemyOK(events []ContentEvent) error {
	var revR, revF, reqC uint64
	var sawR, sawF, sawC bool
	for _, e := range events {
		switch e.Kind {
		case ContentRevoke:
			revR, sawR = e.Rev, true
		case ContentFence:
			revF, sawF = e.Rev, true
		case ContentPublish:
			reqC, sawC = e.Rev, true
		case ContentCheck:
			if !sawC {
				return fmt.Errorf("check before publish")
			}
			if e.Allowed && e.EvalRev < reqC {
				return fmt.Errorf("new-enemy: ALLOW at eval %d < required %d", e.EvalRev, reqC)
			}
		}
	}
	if sawR && sawF && !(revR < revF) {
		return fmt.Errorf("new-enemy: revoke %d not before fence %d", revR, revF)
	}
	if sawF && sawC && revF != reqC {
		return fmt.Errorf("new-enemy: fence %d != publish required %d", revF, reqC)
	}
	return nil
}
