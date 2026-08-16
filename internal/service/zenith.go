package service

import (
	"context"
	"fmt"
	"sync"

	"time"

	"github.com/zenith/zenith/internal/api"
	"github.com/zenith/zenith/internal/cache"
	"github.com/zenith/zenith/internal/circuitbreaker"
	"github.com/zenith/zenith/internal/db"
	"github.com/zenith/zenith/internal/engine"
	"github.com/zenith/zenith/internal/metrics"
	"github.com/zenith/zenith/internal/models"
	"github.com/zenith/zenith/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service implements the Zenith gRPC service
type Service struct {
	api.UnimplementedZenithServer
	repo            *db.TupleRepo
	expander        *engine.ExpansionEngine
	reverseExpander *engine.ReverseExpansionEngine
	cache           *cache.Cache                   // Optional cache
	sfGroup         singleflight.Group             // For request deduplication
	sfMu            sync.Mutex                     // Protects sfGroup (though singleflight is thread-safe)
	circuitBreaker  *circuitbreaker.CircuitBreaker // Optional circuit breaker
}

// NewService creates a new Zenith service
func NewService(repo *db.TupleRepo, expander *engine.ExpansionEngine) *Service {
	reverseExpander := engine.NewReverseExpansionEngine(repo, 10, 100)
	return &Service{
		repo:            repo,
		expander:        expander,
		reverseExpander: reverseExpander,
	}
}

// NewServiceWithCache creates a new Zenith service with caching
func NewServiceWithCache(repo *db.TupleRepo, expander *engine.ExpansionEngine, cache *cache.Cache) *Service {
	reverseExpander := engine.NewReverseExpansionEngineWithCache(repo, cache, 10, 100)
	return &Service{
		repo:            repo,
		expander:        expander,
		reverseExpander: reverseExpander,
		cache:           cache,
	}
}

// NewServiceWithCircuitBreaker creates a new Zenith service with circuit breaker
func NewServiceWithCircuitBreaker(repo *db.TupleRepo, expander *engine.ExpansionEngine, cache *cache.Cache, cb *circuitbreaker.CircuitBreaker) *Service {
	reverseExpander := engine.NewReverseExpansionEngineWithCache(repo, cache, 10, 100)
	return &Service{
		repo:            repo,
		expander:        expander,
		reverseExpander: reverseExpander,
		cache:           cache,
		circuitBreaker:  cb,
	}
}

// checkKey generates a unique key for a Check request for deduplication
func checkKey(req *api.CheckRequest) string {
	return fmt.Sprintf("check:%s:%s:%s:%s:%s:%s:%d",
		req.SubjectNamespace,
		req.SubjectId,
		req.SubjectRelation,
		req.Namespace,
		req.ObjectId,
		req.Relation,
		req.RequiredZookie,
	)
}

// Write handles Write RPC requests
func (s *Service) Write(ctx context.Context, req *api.WriteRequest) (*api.WriteResponse, error) {
	start := time.Now()
	// Create span for Write operation
	ctx, span := observability.StartSpan(ctx, "Write",
		trace.WithAttributes(
			attribute.String("operation", req.Operation.String()),
		),
	)
	defer span.End()
	defer func() {
		duration := time.Since(start)
		metrics.RecordRequestDuration("write", duration)
	}()

	if req.Tuple == nil {
		span.RecordError(status.Error(codes.InvalidArgument, "tuple is required"))
		return nil, status.Error(codes.InvalidArgument, "tuple is required")
	}

	// Convert protobuf tuple to internal model
	tuple := &models.Tuple{
		Namespace:        req.Tuple.Namespace,
		ObjectID:         req.Tuple.ObjectId,
		Relation:         req.Tuple.Relation,
		SubjectNamespace: req.Tuple.SubjectNamespace,
		SubjectID:        req.Tuple.SubjectId,
		SubjectRelation:  req.Tuple.SubjectRelation, // Empty string for direct users
	}

	// Add tuple info to span
	span.SetAttributes(
		attribute.String("tuple.namespace", tuple.Namespace),
		attribute.String("tuple.object_id", tuple.ObjectID),
		attribute.String("tuple.relation", tuple.Relation),
		attribute.String("tuple.subject_namespace", tuple.SubjectNamespace),
		attribute.String("tuple.subject_id", tuple.SubjectID),
	)

	// Validate tuple
	if err := tuple.Validate(); err != nil {
		span.RecordError(err)
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid tuple: %v", err))
	}

	var zookie int64
	var err error

	switch req.Operation {
	case api.WriteOperation_WRITE_OPERATION_INSERT:
		zookie, err = s.repo.Insert(ctx, tuple)
		if err != nil {
			span.RecordError(err)
			metrics.RecordRequest("write", "error")
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to insert tuple: %v", err))
		}
		metrics.RecordRequest("write", "success")

	case api.WriteOperation_WRITE_OPERATION_DELETE:
		zookie, err = s.repo.Delete(ctx, tuple)
		if err != nil {
			span.RecordError(err)
			metrics.RecordRequest("write", "error")
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to delete tuple: %v", err))
		}
		metrics.RecordRequest("write", "success")

	case api.WriteOperation_WRITE_OPERATION_UNSPECIFIED:
		err := status.Error(codes.InvalidArgument, "operation must be specified")
		span.RecordError(err)
		return nil, err

	default:
		err := status.Error(codes.InvalidArgument, fmt.Sprintf("unknown operation: %v", req.Operation))
		span.RecordError(err)
		return nil, err
	}

	// Add zookie to span
	observability.AddZookieToSpan(span, zookie)

	// Invalidate cache on write to maintain consistency
	if s.cache != nil {
		// Invalidate all cache entries related to this tuple
		s.cache.InvalidateRelated(
			tuple.Namespace,
			tuple.ObjectID,
			tuple.Relation,
			tuple.SubjectNamespace,
			tuple.SubjectID,
			tuple.SubjectRelation,
		)
	}

	return &api.WriteResponse{
		Zookie: zookie,
	}, nil
}

// Check handles Check RPC requests
// Phase 4: Recursive expansion with userset resolution, singleflight deduplication
func (s *Service) Check(ctx context.Context, req *api.CheckRequest) (*api.CheckResponse, error) {
	start := time.Now()
	// Validate required fields
	if req.Namespace == "" {
		metrics.RecordRequest("check", "invalid_argument")
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.ObjectId == "" {
		metrics.RecordRequest("check", "invalid_argument")
		return nil, status.Error(codes.InvalidArgument, "object_id is required")
	}
	if req.Relation == "" {
		metrics.RecordRequest("check", "invalid_argument")
		return nil, status.Error(codes.InvalidArgument, "relation is required")
	}
	if req.SubjectNamespace == "" {
		metrics.RecordRequest("check", "invalid_argument")
		return nil, status.Error(codes.InvalidArgument, "subject_namespace is required")
	}
	if req.SubjectId == "" {
		metrics.RecordRequest("check", "invalid_argument")
		return nil, status.Error(codes.InvalidArgument, "subject_id is required")
	}

	// Create span for Check operation
	ctx, span := observability.StartSpan(ctx, "CheckRequest",
		trace.WithAttributes(
			attribute.String("check.subject_namespace", req.SubjectNamespace),
			attribute.String("check.subject_id", req.SubjectId),
			attribute.String("check.namespace", req.Namespace),
			attribute.String("check.object_id", req.ObjectId),
			attribute.String("check.relation", req.Relation),
		),
	)
	defer span.End()
	defer func() {
		duration := time.Since(start)
		metrics.RecordRequestDuration("check", duration)
	}()

	if req.RequiredZookie > 0 {
		observability.AddZookieToSpan(span, req.RequiredZookie)
		span.SetAttributes(attribute.String("check.required_zookie", fmt.Sprintf("%d", req.RequiredZookie)))
	}

	// Check circuit breaker if enabled
	if s.circuitBreaker != nil {
		if !s.circuitBreaker.Allow(ctx) {
			// Circuit is open, use stale cache fallback
			span.SetAttributes(attribute.String("circuit_breaker.state", "open"))
			return s.checkStale(ctx, req, span)
		}
		span.SetAttributes(attribute.String("circuit_breaker.state", s.circuitBreaker.State().String()))
	}

	// Use singleflight to deduplicate concurrent identical requests
	key := checkKey(req)
	result, err, shared := s.sfGroup.Do(key, func() (interface{}, error) {
		// Build check request for expansion engine
		checkReq := &engine.CheckRequest{
			SubjectNamespace: req.SubjectNamespace,
			SubjectID:        req.SubjectId,
			SubjectRelation:  req.SubjectRelation, // Empty string for direct users
			ObjectNamespace:  req.Namespace,
			ObjectID:         req.ObjectId,
			Relation:         req.Relation,
			RequiredZookie:   req.RequiredZookie,
		}

		// Perform recursive check with userset expansion
		allowed, zookie, err := s.expander.Check(ctx, checkReq)
		if err != nil {
			// Record failure in circuit breaker
			if s.circuitBreaker != nil {
				s.circuitBreaker.RecordFailure()
			}
			// Handle timeout gracefully (return false, not error)
			if err.Error() == "check timeout exceeded" {
				return &api.CheckResponse{
					Allowed: false,
					Zookie:  0,
				}, nil
			}
			return nil, err
		}

		// Record success in circuit breaker
		if s.circuitBreaker != nil {
			s.circuitBreaker.RecordSuccess()
		}

		return &api.CheckResponse{
			Allowed: allowed,
			Zookie:  zookie,
		}, nil
	})

	// Add deduplication info to span
	if shared {
		span.SetAttributes(attribute.Bool("check.deduplicated", true))
	}

	if err != nil {
		span.RecordError(err)
		metrics.RecordRequest("check", "error")
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to check tuple: %v", err))
	}

	resp := result.(*api.CheckResponse)

	// Add result to span
	span.SetAttributes(
		attribute.Bool("check.allowed", resp.Allowed),
	)
	if resp.Zookie > 0 {
		observability.AddZookieToSpan(span, resp.Zookie)
	}

	if resp.Allowed {
		metrics.RecordRequest("check", "allowed")
	} else {
		metrics.RecordRequest("check", "denied")
	}

	return resp, nil
}

// checkStale performs a check using stale cache when circuit breaker is open
// This bypasses zookie validation and uses cached results even if expired
func (s *Service) checkStale(ctx context.Context, req *api.CheckRequest, span trace.Span) (*api.CheckResponse, error) {
	span.SetAttributes(attribute.Bool("check.stale_mode", true))

	// Try to get from cache without zookie validation
	if s.cache != nil {
		// Use a modified key that doesn't include zookie for stale lookups
		staleKey := fmt.Sprintf("check:%s:%s:%s:%s:%s:%s",
			req.SubjectNamespace,
			req.SubjectId,
			req.SubjectRelation,
			req.Namespace,
			req.ObjectId,
			req.Relation,
		)

		// Try to get from cache (even if expired)
		entry, ok := s.cache.GetCheckWithContext(ctx, staleKey)
		if ok {
			span.SetAttributes(attribute.Bool("check.stale_cache_hit", true))
			metrics.RecordRequest("check", "stale_cache_hit")
			return &api.CheckResponse{
				Allowed: entry.Allowed,
				Zookie:  entry.Zookie, // May be stale, but better than nothing
			}, nil
		}
	}

	// No cache available, return denied (safe default)
	span.SetAttributes(attribute.Bool("check.stale_cache_miss", true))
	metrics.RecordRequest("check", "stale_cache_miss")
	return &api.CheckResponse{
		Allowed: false,
		Zookie:  0,
	}, nil
}

// ListSubjects handles ListSubjects RPC requests
// Uses reverse expansion to find all subjects with a relation on an object
func (s *Service) ListSubjects(ctx context.Context, req *api.ListSubjectsRequest) (*api.ListSubjectsResponse, error) {
	// Validate required fields
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.ObjectId == "" {
		return nil, status.Error(codes.InvalidArgument, "object_id is required")
	}
	if req.Relation == "" {
		return nil, status.Error(codes.InvalidArgument, "relation is required")
	}

	// Create span for ListSubjects operation
	ctx, span := observability.StartSpan(ctx, "ListSubjects",
		trace.WithAttributes(
			attribute.String("list.namespace", req.Namespace),
			attribute.String("list.object_id", req.ObjectId),
			attribute.String("list.relation", req.Relation),
		),
	)
	defer span.End()

	if req.RequiredZookie > 0 {
		observability.AddZookieToSpan(span, req.RequiredZookie)
	}

	// Perform reverse expansion
	subjects, zookie, err := s.reverseExpander.ListSubjects(ctx, req.Namespace, req.ObjectId, req.Relation, req.RequiredZookie)
	if err != nil {
		span.RecordError(err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to list subjects: %v", err))
	}

	// Convert internal subjects to protobuf subjects
	pbSubjects := make([]*api.Subject, 0, len(subjects))
	for _, subject := range subjects {
		pbSubjects = append(pbSubjects, &api.Subject{
			Namespace: subject.Namespace,
			Id:        subject.ID,
			Relation:  subject.Relation,
		})
	}

	span.SetAttributes(
		attribute.Int("list.subject_count", len(pbSubjects)),
	)
	if zookie > 0 {
		observability.AddZookieToSpan(span, zookie)
	}

	return &api.ListSubjectsResponse{
		Subjects: pbSubjects,
		Zookie:   zookie,
	}, nil
}

// BatchCheck handles BatchCheck RPC requests
// Processes multiple permission checks in parallel (up to 30)
func (s *Service) BatchCheck(ctx context.Context, req *api.BatchCheckRequest) (*api.BatchCheckResponse, error) {
	start := time.Now()

	// Create span for BatchCheck operation
	ctx, span := observability.StartSpan(ctx, "BatchCheck",
		trace.WithAttributes(
			attribute.Int("batch.size", len(req.Requests)),
		),
	)
	defer span.End()
	defer func() {
		duration := time.Since(start)
		metrics.RecordRequestDuration("batch_check", duration)
		metrics.RecordBatchCheckSize(len(req.Requests))
	}()

	// Validate request size (max 30 checks)
	if len(req.Requests) == 0 {
		span.RecordError(status.Error(codes.InvalidArgument, "at least one check request is required"))
		metrics.RecordRequest("batch_check", "invalid_argument")
		return nil, status.Error(codes.InvalidArgument, "at least one check request is required")
	}
	if len(req.Requests) > 30 {
		span.RecordError(status.Error(codes.InvalidArgument, "maximum 30 checks allowed per batch"))
		metrics.RecordRequest("batch_check", "invalid_argument")
		return nil, status.Error(codes.InvalidArgument, "maximum 30 checks allowed per batch")
	}

	// Validate each request
	for i, checkReq := range req.Requests {
		if checkReq.Namespace == "" {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("request %d: namespace is required", i))
		}
		if checkReq.ObjectId == "" {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("request %d: object_id is required", i))
		}
		if checkReq.Relation == "" {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("request %d: relation is required", i))
		}
		if checkReq.SubjectNamespace == "" {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("request %d: subject_namespace is required", i))
		}
		if checkReq.SubjectId == "" {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("request %d: subject_id is required", i))
		}
	}

	// Use required_zookie from batch request if provided, otherwise use individual request zookies
	requiredZookie := req.RequiredZookie
	if requiredZookie == 0 {
		// If batch-level zookie not provided, use max from individual requests
		for _, checkReq := range req.Requests {
			if checkReq.RequiredZookie > requiredZookie {
				requiredZookie = checkReq.RequiredZookie
			}
		}
	}

	if requiredZookie > 0 {
		observability.AddZookieToSpan(span, requiredZookie)
		span.SetAttributes(attribute.String("batch.required_zookie", fmt.Sprintf("%d", requiredZookie)))
	}

	// Process checks in parallel using goroutines
	type checkResult struct {
		index  int
		result *api.CheckResult
		err    error
	}

	resultChan := make(chan checkResult, len(req.Requests))
	var wg sync.WaitGroup

	// Launch goroutine for each check
	for i, checkReq := range req.Requests {
		wg.Add(1)
		go func(idx int, cr *api.CheckRequest) {
			defer wg.Done()

			// Use individual required_zookie if batch-level not provided
			checkZookie := requiredZookie
			if checkZookie == 0 && cr.RequiredZookie > 0 {
				checkZookie = cr.RequiredZookie
			}

			// Create a CheckRequest with the zookie
			checkRequest := &api.CheckRequest{
				SubjectNamespace: cr.SubjectNamespace,
				SubjectId:        cr.SubjectId,
				SubjectRelation:  cr.SubjectRelation,
				Namespace:        cr.Namespace,
				ObjectId:         cr.ObjectId,
				Relation:         cr.Relation,
				RequiredZookie:   checkZookie,
			}

			// Use singleflight to deduplicate identical checks within the batch
			key := checkKey(checkRequest)
			result, err, _ := s.sfGroup.Do(key, func() (interface{}, error) {
				// Build check request for expansion engine
				checkReq := &engine.CheckRequest{
					SubjectNamespace: checkRequest.SubjectNamespace,
					SubjectID:        checkRequest.SubjectId,
					SubjectRelation:  checkRequest.SubjectRelation,
					ObjectNamespace:  checkRequest.Namespace,
					ObjectID:         checkRequest.ObjectId,
					Relation:         checkRequest.Relation,
					RequiredZookie:   checkRequest.RequiredZookie,
				}

				// Perform recursive check with userset expansion
				allowed, zookie, err := s.expander.Check(ctx, checkReq)
				if err != nil {
					// Handle timeout gracefully (return false, not error)
					if err.Error() == "check timeout exceeded" {
						return &api.CheckResult{
							Allowed: false,
							Zookie:  0,
						}, nil
					}
					return nil, err
				}

				return &api.CheckResult{
					Allowed: allowed,
					Zookie:  zookie,
				}, nil
			})

			if err != nil {
				resultChan <- checkResult{
					index: idx,
					err:   err,
				}
				return
			}

			resultChan <- checkResult{
				index:  idx,
				result: result.(*api.CheckResult),
			}
		}(i, checkReq)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results (may arrive out of order)
	results := make([]*api.CheckResult, len(req.Requests))
	var maxZookie int64
	var hasError bool
	var firstError error

	for res := range resultChan {
		if res.err != nil {
			hasError = true
			if firstError == nil {
				firstError = res.err
			}
			// For errors, return a denied result with zookie 0
			results[res.index] = &api.CheckResult{
				Allowed: false,
				Zookie:  0,
			}
		} else {
			results[res.index] = res.result
			if res.result.Zookie > maxZookie {
				maxZookie = res.result.Zookie
			}
		}
	}

	// If we got a zookie of 0 from all checks, get current zookie
	if maxZookie == 0 {
		zookie, err := s.repo.GetZookie(ctx)
		if err != nil {
			span.RecordError(err)
			metrics.RecordRequest("batch_check", "error")
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get zookie: %v", err))
		}
		maxZookie = zookie
	}

	// Add result summary to span
	allowedCount := 0
	for _, res := range results {
		if res != nil && res.Allowed {
			allowedCount++
		}
	}
	span.SetAttributes(
		attribute.Int("batch.allowed_count", allowedCount),
		attribute.Int("batch.denied_count", len(results)-allowedCount),
	)
	if maxZookie > 0 {
		observability.AddZookieToSpan(span, maxZookie)
	}

	if hasError {
		metrics.RecordRequest("batch_check", "partial_error")
		// Return partial results with error (client can check individual results)
		// For now, we'll return the results but log the error
		span.RecordError(firstError)
	} else {
		metrics.RecordRequest("batch_check", "success")
	}

	return &api.BatchCheckResponse{
		Results: results,
		Zookie:  maxZookie,
	}, nil
}

// ListAllTuples returns all tuples in the database
// This is a helper method for the REST gateway to list all tuples
func (s *Service) ListAllTuples(ctx context.Context) ([]*models.Tuple, error) {
	return s.repo.ListAll(ctx)
}
