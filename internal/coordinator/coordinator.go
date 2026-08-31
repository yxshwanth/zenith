// Package coordinator selects a read revision under an explicit consistency
// mode (docs/adr/0003-logical-revision-as-clock.md). It does not evaluate permissions.
package coordinator

import (
	"fmt"

	v2 "github.com/yxshwanth/zenith/api/zenith/v2"
	"github.com/yxshwanth/zenith/internal/mvcc"
	"github.com/yxshwanth/zenith/internal/token"
)

// Applied is the locally materialized applied revision.
// Floor is the MVCC history floor (compacted below this).
type State struct {
	Applied mvcc.Revision
	Floor   mvcc.Revision
}

// SelectRevision chooses the snapshot revision for a Check.
// FullyConsistent expects the caller to have already committed a fence
// so Applied is the fence revision.
func SelectRevision(mode v2.Consistency, st State, tok *token.Claims) (mvcc.Revision, error) {
	mode.Normalize()
	switch mode.Mode {
	case v2.FullyConsistent:
		if st.Applied < st.Floor {
			return 0, fmt.Errorf("coordinator: applied below floor")
		}
		return st.Applied, nil
	case v2.AtLeastAsFresh:
		min := mvcc.Revision(mode.Revision)
		if tok != nil && mvcc.Revision(tok.Revision) > min {
			min = mvcc.Revision(tok.Revision)
		}
		if st.Applied < min {
			return 0, fmt.Errorf("coordinator: replica behind required %d (applied %d)", min, st.Applied)
		}
		// Choose applied (newest retained satisfying minimum).
		if st.Applied < st.Floor {
			return 0, fmt.Errorf("coordinator: compacted")
		}
		return st.Applied, nil
	case v2.AtExactRevision:
		r := mvcc.Revision(mode.Revision)
		if tok != nil && r < mvcc.Revision(tok.Revision) {
			return 0, fmt.Errorf("coordinator: exact revision %d below required token %d", r, tok.Revision)
		}
		if r < st.Floor {
			return 0, fmt.Errorf("coordinator: revision compacted: %d", r)
		}
		if r > st.Applied {
			return 0, fmt.Errorf("coordinator: revision not materialized: %d", r)
		}
		return r, nil
	default:
		return st.Applied, nil
	}
}
