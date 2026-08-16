package gateway

// JSON request/response models matching frontend TypeScript types

// RelationTuple represents a permission relationship
type RelationTuple struct {
	Namespace        string `json:"namespace"`
	ObjectID         string `json:"object_id"`
	Relation         string `json:"relation"`
	SubjectNamespace string `json:"subject_namespace"`
	SubjectID        string `json:"subject_id"`
	SubjectRelation  string `json:"subject_relation"`
}

// WriteRequest represents a tuple write operation
type WriteRequest struct {
	Tuple     RelationTuple `json:"tuple"`
	Operation string        `json:"operation"` // "insert" or "delete"
}

// WriteResponse returns the zookie after a write
type WriteResponse struct {
	Zookie string `json:"zookie"`
}

// CheckRequest represents a permission check request
type CheckRequest struct {
	SubjectNamespace string `json:"subject_namespace"`
	SubjectID        string `json:"subject_id"`
	SubjectRelation  string `json:"subject_relation,omitempty"`
	Namespace        string `json:"namespace"`
	ObjectID         string `json:"object_id"`
	Relation         string `json:"relation"`
	RequiredZookie   string `json:"required_zookie,omitempty"`
}

// CheckResponse represents a permission check result
type CheckResponse struct {
	Allowed       bool    `json:"allowed"`
	Zookie        string  `json:"zookie"`
	LatencyMS     float64 `json:"latency_ms,omitempty"`
	CacheHit      bool    `json:"cache_hit,omitempty"`
	ExpansionDepth int    `json:"expansion_depth,omitempty"`
}

// CheckResult represents a single check result in a batch
type CheckResult struct {
	Allowed bool   `json:"allowed"`
	Zookie  string `json:"zookie"`
}

// BatchCheckRequest represents a batch check request
type BatchCheckRequest struct {
	Requests      []CheckRequest `json:"requests"`
	RequiredZookie string         `json:"required_zookie,omitempty"`
}

// BatchCheckResponse represents batch check results
type BatchCheckResponse struct {
	Results      []CheckResult `json:"results"`
	Zookie       string        `json:"zookie"`
	TotalLatency float64       `json:"total_latency,omitempty"`
}

// Subject represents a subject in ListSubjects response
type Subject struct {
	Namespace string `json:"namespace"`
	ID        string `json:"id"`
	Relation  string `json:"relation"`
}

// ListSubjectsResponse represents the response from ListSubjects
type ListSubjectsResponse struct {
	Subjects []Subject `json:"subjects"`
	Zookie   string    `json:"zookie"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

