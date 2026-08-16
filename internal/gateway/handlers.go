package gateway

import (
	"net/http"
	"strings"
	"time"

	"github.com/zenith/zenith/internal/api"
	"google.golang.org/grpc/status"
)

// handleTuples handles tuple CRUD operations
// GET /api/tuples - List all tuples
// POST /api/tuples - Create tuple
func (g *Gateway) handleTuples(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		g.handleListTuples(w, r)
	case http.MethodPost:
		g.handleWriteTuple(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
	}
}

// handleListTuples lists all tuples (helper endpoint for graph visualization)
func (g *Gateway) handleListTuples(w http.ResponseWriter, r *http.Request) {
	// Access the repository through the service
	// We need to add a method to service or access repo directly
	// For now, we'll access the repo through a helper method
	// This requires exposing the repo or adding a ListAll method to service
	
	// Get tuples from service
	ctx := r.Context()
	tuples, err := g.service.ListAllTuples(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list tuples", err.Error())
		return
	}

	// Convert to JSON response
	jsonTuples := make([]RelationTuple, len(tuples))
	for i, tuple := range tuples {
		jsonTuples[i] = RelationTuple{
			Namespace:        tuple.Namespace,
			ObjectID:         tuple.ObjectID,
			Relation:         tuple.Relation,
			SubjectNamespace: tuple.SubjectNamespace,
			SubjectID:        tuple.SubjectID,
			SubjectRelation:  tuple.SubjectRelation,
		}
	}

	writeJSON(w, http.StatusOK, jsonTuples)
}

// handleWriteTuple handles tuple write operations (insert/delete)
func (g *Gateway) handleWriteTuple(w http.ResponseWriter, r *http.Request) {
	var req WriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Convert to gRPC request
	grpcReq := &api.WriteRequest{
		Tuple: &api.RelationTuple{
			Namespace:        req.Tuple.Namespace,
			ObjectId:         req.Tuple.ObjectID,
			Relation:         req.Tuple.Relation,
			SubjectNamespace: req.Tuple.SubjectNamespace,
			SubjectId:        req.Tuple.SubjectID,
			SubjectRelation:  req.Tuple.SubjectRelation,
		},
	}

	// Set operation
	switch strings.ToLower(req.Operation) {
	case "insert":
		grpcReq.Operation = api.WriteOperation_WRITE_OPERATION_INSERT
	case "delete":
		grpcReq.Operation = api.WriteOperation_WRITE_OPERATION_DELETE
	default:
		writeError(w, http.StatusBadRequest, "Invalid operation", "Operation must be 'insert' or 'delete'")
		return
	}

	// Call gRPC service
	ctx := r.Context()
	grpcResp, err := g.service.Write(ctx, grpcReq)
	if err != nil {
		handleGRPCError(w, err)
		return
	}

	// Convert response
	resp := WriteResponse{
		Zookie: formatZookie(grpcResp.Zookie),
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleCheck handles permission check requests
func (g *Gateway) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	var req CheckRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Convert to gRPC request
	requiredZookie, err := parseZookie(req.RequiredZookie)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid required_zookie", err.Error())
		return
	}

	grpcReq := &api.CheckRequest{
		SubjectNamespace: req.SubjectNamespace,
		SubjectId:        req.SubjectID,
		SubjectRelation:  req.SubjectRelation,
		Namespace:        req.Namespace,
		ObjectId:         req.ObjectID,
		Relation:         req.Relation,
		RequiredZookie:   requiredZookie,
	}

	// Call gRPC service
	ctx := r.Context()
	start := time.Now()
	grpcResp, err := g.service.Check(ctx, grpcReq)
	latency := time.Since(start).Seconds() * 1000 // Convert to milliseconds

	if err != nil {
		handleGRPCError(w, err)
		return
	}

	// Convert response
	resp := CheckResponse{
		Allowed:   grpcResp.Allowed,
		Zookie:    formatZookie(grpcResp.Zookie),
		LatencyMS: latency,
		// Note: cache_hit and expansion_depth would need to be added to gRPC response
		// or tracked separately
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleBatchCheck handles batch permission check requests
func (g *Gateway) handleBatchCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	var req BatchCheckRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate request size
	if len(req.Requests) == 0 {
		writeError(w, http.StatusBadRequest, "No requests provided", "At least one check request is required")
		return
	}
	if len(req.Requests) > 30 {
		writeError(w, http.StatusBadRequest, "Too many requests", "Maximum 30 checks allowed per batch")
		return
	}

	// Convert to gRPC request
	requiredZookie, err := parseZookie(req.RequiredZookie)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid required_zookie", err.Error())
		return
	}

	grpcRequests := make([]*api.CheckRequest, len(req.Requests))
	for i, checkReq := range req.Requests {
		checkZookie, _ := parseZookie(checkReq.RequiredZookie)
		if requiredZookie > 0 {
			checkZookie = requiredZookie
		}
		grpcRequests[i] = &api.CheckRequest{
			SubjectNamespace: checkReq.SubjectNamespace,
			SubjectId:        checkReq.SubjectID,
			SubjectRelation:  checkReq.SubjectRelation,
			Namespace:        checkReq.Namespace,
			ObjectId:         checkReq.ObjectID,
			Relation:         checkReq.Relation,
			RequiredZookie:   checkZookie,
		}
	}

	grpcReq := &api.BatchCheckRequest{
		Requests:      grpcRequests,
		RequiredZookie: requiredZookie,
	}

	// Call gRPC service
	ctx := r.Context()
	start := time.Now()
	grpcResp, err := g.service.BatchCheck(ctx, grpcReq)
	totalLatency := time.Since(start).Seconds() * 1000

	if err != nil {
		handleGRPCError(w, err)
		return
	}

	// Convert response
	results := make([]CheckResult, len(grpcResp.Results))
	for i, result := range grpcResp.Results {
		results[i] = CheckResult{
			Allowed: result.Allowed,
			Zookie:  formatZookie(result.Zookie),
		}
	}

	resp := BatchCheckResponse{
		Results:      results,
		Zookie:       formatZookie(grpcResp.Zookie),
		TotalLatency: totalLatency,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleListSubjects handles ListSubjects requests
// GET /api/subjects/:namespace/:object_id/:relation
func (g *Gateway) handleListSubjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "Use GET")
		return
	}

	// Parse path: /api/subjects/:namespace/:object_id/:relation
	trimmedPath := strings.TrimPrefix(r.URL.Path, "/api/subjects/")
	if trimmedPath == r.URL.Path {
		writeError(w, http.StatusBadRequest, "Invalid path", 
			"Expected: /api/subjects/:namespace/:object_id/:relation")
		return
	}

	pathParts := strings.Split(trimmedPath, "/")
	if len(pathParts) != 3 {
		writeError(w, http.StatusBadRequest, "Invalid path", 
			"Expected: /api/subjects/:namespace/:object_id/:relation")
		return
	}

	namespace := pathParts[0]
	objectID := pathParts[1]
	relation := pathParts[2]

	// Parse query parameters
	requiredZookieStr := r.URL.Query().Get("required_zookie")
	requiredZookie, err := parseZookie(requiredZookieStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid required_zookie", err.Error())
		return
	}

	// Convert to gRPC request
	grpcReq := &api.ListSubjectsRequest{
		Namespace:      namespace,
		ObjectId:        objectID,
		Relation:        relation,
		RequiredZookie:  requiredZookie,
	}

	// Call gRPC service
	ctx := r.Context()
	grpcResp, err := g.service.ListSubjects(ctx, grpcReq)
	if err != nil {
		handleGRPCError(w, err)
		return
	}

	// Convert response
	subjects := make([]Subject, len(grpcResp.Subjects))
	for i, subj := range grpcResp.Subjects {
		subjects[i] = Subject{
			Namespace: subj.Namespace,
			ID:        subj.Id,
			Relation:  subj.Relation,
		}
	}

	resp := ListSubjectsResponse{
		Subjects: subjects,
		Zookie:   formatZookie(grpcResp.Zookie),
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleHealth handles health check requests
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGRPCError converts gRPC errors to HTTP responses
func handleGRPCError(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Internal server error", err.Error())
		return
	}

	httpStatus := grpcToHTTPStatus(st.Code())
	detail := ""
	if len(st.Details()) > 0 {
		if str, ok := st.Details()[0].(string); ok {
			detail = str
		}
	}
	writeError(w, httpStatus, st.Message(), detail)
}

