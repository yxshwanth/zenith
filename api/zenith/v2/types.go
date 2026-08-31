// Package v2 defines the Zenith v2 client API surface (UPDATE.md §12).
// Hand-written types for in-process use; generated stubs live in ./pb (make generate).
package v2

// ConsistencyMode selects a read contract. Omitted / zero => FullyConsistent.
type ConsistencyMode int

const (
	FullyConsistent ConsistencyMode = iota
	AtLeastAsFresh
	AtExactRevision
)

// Consistency is mutually exclusive modes.
type Consistency struct {
	Mode     ConsistencyMode
	Revision uint64 // used by AtLeastAsFresh / AtExactRevision
}

// Normalize sets omitted mode to FullyConsistent.
func (c *Consistency) Normalize() {
	if c == nil {
		return
	}
}

// CheckRequest is one permission check.
type CheckRequest struct {
	Store            string
	ObjectNamespace  string
	ObjectID         string
	Relation         string
	SubjectNamespace string
	SubjectID        string
	Consistency      Consistency
	RequiredToken    string
}

// CheckResponse is ALLOW/DENY plus evaluation token, or typed error.
type CheckResponse struct {
	Allowed bool
	Token   string
	Err     error
}

// WriteTuplesRequest is an atomic bounded batch.
type WriteTuplesRequest struct {
	Store     string
	Session   string
	Seq       uint64
	Tuples    []Tuple
	Deletes   []Tuple
	RequestID string
}

// Tuple mirrors the relation-tuple vocabulary.
type Tuple struct {
	ObjectNamespace  string
	ObjectID         string
	Relation         string
	SubjectNamespace string
	SubjectID        string
	SubjectRelation  string
}

// WriteTuplesResponse returns the committed revision token.
type WriteTuplesResponse struct {
	Token string
	Rev   uint64
	Err   error
}

// BatchCheckRequest shares one snapshot across items.
type BatchCheckRequest struct {
	Store       string
	Consistency Consistency
	Items       []CheckRequest
}

// BatchCheckResponse reports one revision for all items.
type BatchCheckResponse struct {
	Revision uint64
	Token    string
	Results  []CheckResponse
	Err      error
}

// CheckContentChangeRequest fences a writer before publication.
type CheckContentChangeRequest struct {
	Store            string
	ObjectNamespace  string
	ObjectID         string
	Relation         string
	SubjectNamespace string
	SubjectID        string
	RequestID        string
}

// CheckContentChangeResponse returns a fence-backed token only on ALLOW.
type CheckContentChangeResponse struct {
	Allowed bool
	Token   string
	Rev     uint64
	Err     error
}

// ListSubjectsRequest enumerates subjects at one snapshot.
type ListSubjectsRequest struct {
	Store           string
	ObjectNamespace string
	ObjectID        string
	Relation        string
	Consistency     Consistency
	Limit           int
}

// Subject is one enumerated subject.
type Subject struct {
	SubjectNamespace string
	SubjectID        string
	SubjectRelation  string
}

// ListSubjectsResponse is complete-or-error (no speculative stream).
type ListSubjectsResponse struct {
	Subjects []Subject
	Token    string
	Rev      uint64
	Complete bool
	Err      error
}

// GetRequestOutcomeRequest looks up a sessioned mutation.
type GetRequestOutcomeRequest struct {
	Session   string
	Seq       uint64
	RequestID string
}

// GetRequestOutcomeResponse returns the stored outcome if present.
type GetRequestOutcomeResponse struct {
	Found  bool
	Rev    uint64
	Result string
	Err    error
}

// GetClusterStatusResponse is diagnostics.
type GetClusterStatusResponse struct {
	Role         string
	Term         uint64
	CommitIndex  uint64
	AppliedIndex uint64
}
