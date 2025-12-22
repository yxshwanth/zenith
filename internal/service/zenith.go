package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/zenith/zenith/internal/api"
	"github.com/zenith/zenith/internal/cache"
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
	"time"
)

// Service implements the Zenith gRPC service
type Service struct {
	api.UnimplementedZenithServer
	repo            *db.TupleRepo
	expander        *engine.ExpansionEngine
	reverseExpander *engine.ReverseExpansionEngine
	cache           *cache.Cache // Optional cache
	sfGroup         singleflight.Group // For request deduplication
	sfMu            sync.Mutex         // Protects sfGroup (though singleflight is thread-safe)
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
			// Handle timeout gracefully (return false, not error)
			if err.Error() == "check timeout exceeded" {
				return &api.CheckResponse{
					Allowed: false,
					Zookie:  0,
				}, nil
			}
			return nil, err
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

